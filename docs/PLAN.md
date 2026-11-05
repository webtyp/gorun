---
PLAN: "fix: stop kills only processes of the same executable path, never every process with the same name"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 6426112914768762331
PR: https://github.com/webtyp/gorun/pull/5
REVIEWED_COMMIT: dd9b93509376d8e5013b432038ebb0af3138a794
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.

# gorun — kill by executable path, not by name

## Why

`webtyp dev` will run up to 5 projects at once. Every project's dev server binary is called `server`
(`<project>/web/server/server`). Today, with `KillAllOnStop: true`, `RunProgram` and `StopProgram` call
`KillAllByName("server")`, which runs `pgrep -x server` and kills **every** process named `server` on the machine:
starting or stopping project B kills project A's server. The cleanup exists to kill orphans of **this** binary left
by a crashed parent — so it must match the executable's **absolute path**, which is unique per project.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

## Decisions (no judgment needed)

- New unexported `killAllByPath(executablePath string) error` replaces the kill-by-name path.
- **Deleted** (exported API, no consumer left once `webtyp/devflow` drops `StopApp` — that change ships separately):
  `StopApp`, `KillAllByName`, `killAllUnix`, `killAllWindows`. Criterion:
  `grep -rn "StopApp\|KillAllByName\|killAllUnix\|killAllWindows" --include=*.go .` → empty.
- `DisableGlobalCleanup` keeps its name and meaning (true → no cleanup of other instances); only its comment changes.
- Platform listing of PIDs by executable path lives in three files with build tags (the API differs per OS):
  `cleanup_linux.go`, `cleanup_darwin.go`, `cleanup_windows.go`, plus `cleanup_other.go` for every other OS.
- A process is never killed if it is this process (`os.Getpid()`) or its parent (`os.Getppid()`).
- This is backend tooling: stdlib `os/exec`, `fmt`, `path/filepath` are correct here (see AGENTS.md).

## Stage 1 — `cleanup.go`

Replace the whole file content from `// StopApp terminates` down to the end of `killAllUnix` (the functions
`StopApp`, `KillAllByName`, `killAllWindows`, `killAllUnix`) with:

```go
// killAllByPath kills every running process whose executable is exactly
// executablePath, except this process and its parent. Matching the path —
// never the bare name — is what lets two projects run a binary called
// "server" side by side: each one only ever kills its own.
func killAllByPath(executablePath string) error {
	abs, err := filepath.Abs(executablePath)
	if err != nil {
		return fmt.Errorf(errResolvePath, executablePath, err)
	}
	pids, err := listPIDsByExecutable(abs)
	if err != nil {
		return err
	}
	myPid, parentPid := os.Getpid(), os.Getppid()
	var failures []string
	for _, pid := range pids {
		if pid == myPid || pid == parentPid {
			continue
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			failures = append(failures, fmt.Sprintf(errFindProcess, pid, err))
			continue
		}
		if err := killProcessGraceful(process); err != nil {
			failures = append(failures, fmt.Sprintf(errKillProcess, pid, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf(errSomeNotKilled, strings.Join(failures, "; "))
	}
	return nil
}

const (
	errResolvePath   = "gorun: resolve executable path %s: %w"
	errFindProcess   = "failed to find process %d: %v"
	errKillProcess   = "failed to kill process %d: %v"
	errSomeNotKilled = "some processes could not be killed: %s"
)
```

In `stopProgramAndCleanupUnsafe`, replace the block that starts at `// If requested, also kill all other instances`
through the closing `}` of `if killAll && h.ExecProgramPath != ""` with:

```go
	// If requested, also kill other instances of this exact executable
	// (orphans of a parent that died without stopping its child).
	if killAll && h.ExecProgramPath != "" && !h.DisableGlobalCleanup {
		if cleanupErr := killAllByPath(h.ExecProgramPath); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to cleanup other instances of %s: %v\n", h.ExecProgramPath, cleanupErr)
		}
	}
```

Fix the imports (`path/filepath` added; `runtime`, `strconv` removed if unused).

In `goRun.go`, the `DisableGlobalCleanup` comment becomes:
`// If true, never kills other processes of the same executable path, even if KillAllOnStop is true`.

## Stage 2 — platform files

`cleanup_linux.go`:

```go
//go:build linux

package gorun

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// procDir and deletedSuffix are Linux /proc facts: /proc/<pid>/exe links to the
// executable, and reads "<path> (deleted)" once the file was rebuilt under it.
const (
	procDir       = "/proc"
	procExe       = "exe"
	deletedSuffix = " (deleted)"
)

func listPIDsByExecutable(abs string) ([]int, error) {
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // not a process directory
		}
		target, err := os.Readlink(filepath.Join(procDir, e.Name(), procExe))
		if err != nil {
			continue // another user's process, or it just exited
		}
		if strings.TrimSuffix(target, deletedSuffix) == abs {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
```

`cleanup_darwin.go` (`ps -o comm=` prints the full executable path on macOS):

```go
//go:build darwin

package gorun

import (
	"os/exec"
	"strconv"
	"strings"
)

const psCommand = "ps"

var psArgs = []string{"-axo", "pid=,comm="}

func listPIDsByExecutable(abs string) ([]int, error) {
	out, err := exec.Command(psCommand, psArgs...).Output()
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		sep := strings.IndexByte(line, ' ')
		if sep < 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:sep])
		if err != nil {
			continue
		}
		if strings.TrimSpace(line[sep+1:]) == abs {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
```

`cleanup_windows.go`:

```go
//go:build windows

package gorun

import (
	"os/exec"
	"strconv"
	"strings"
)

const powershell = "powershell"

// psFilter lists the PIDs whose ExecutablePath equals %s (a single-quoted
// PowerShell literal: a quote inside the path is doubled).
const psFilter = "Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -eq '%s' } | ForEach-Object { $_.ProcessId }"

func listPIDsByExecutable(abs string) ([]int, error) {
	script := strings.Replace(psFilter, "%s", strings.ReplaceAll(abs, "'", "''"), 1)
	out, err := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(field); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
```

`cleanup_other.go`:

```go
//go:build !linux && !darwin && !windows

package gorun

import (
	"fmt"
	"runtime"
)

const errUnsupportedOS = "gorun: cannot list processes by executable path on %s"

func listPIDsByExecutable(abs string) ([]int, error) {
	return nil, fmt.Errorf(errUnsupportedOS, runtime.GOOS)
}
```

## Stage 3 — tests

- Delete `TestKillAllByName_NonExistent` and `TestStopApp_NonExistent` from `cleanup_test.go`.
- New `cleanup_path_test.go` (package `gorun`, like the existing tests; it reuses `buildTestProgram(t, "long_program")`
  from `gorun_test.go`). Skip on `runtime.GOOS == "windows"` with `t.Skip("process listing covered on linux/darwin")`.

| Test | Setup | Call | Expected |
|---|---|---|---|
| `TestKillAllByPath_SameNameOtherDirSurvives` | build `long_program`; copy the binary to `dirA/server` and `dirB/server` (two `t.TempDir()`, mode 0755); `a := New(&Config{ExecProgramPath: dirA/server, KillAllOnStop: true, ExitChan: make(chan bool)})`, same for `b` with `dirB/server` | `a.RunProgram()`, then `b.RunProgram()`, then `b.StopProgram()` | `a.IsRunning() == true` and `a.GetPID()`'s process still answers `process.Signal(syscall.Signal(0)) == nil`; cleanup: `a.StopProgram()` |
| `TestKillAllByPath_KillsOrphanOfSamePath` | build as above into `dirA/server`; start an orphan directly: `orphan := exec.Command(dirA/server); orphan.Start()` | `a := New(...dirA/server, KillAllOnStop: true...)`; `a.RunProgram()` | within 3 s `orphan.Wait()` returns (the orphan was killed); `a.IsRunning() == true`; cleanup `a.StopProgram()` |
| `TestKillAllByPath_DisableGlobalCleanupSparesOrphan` | as the previous one, with `DisableGlobalCleanup: true` | `a.RunProgram()` | after 500 ms `orphan.Process.Signal(syscall.Signal(0)) == nil` (alive); cleanup: `orphan.Process.Kill()`, `a.StopProgram()` |

## Stage 4 — documentation

- `README.md`: delete the `gorun.StopApp("my-server")` example (line 36) and its surrounding sentence; in its place
  one paragraph: «With `KillAllOnStop`, `RunProgram` and `StopProgram` also kill orphans of the **same executable
  path** (left by a crashed parent). Two programs with the same file name in different folders never touch each
  other. `DisableGlobalCleanup: true` turns that off.»
- Bring `README.md` to the four sections of the ecosystem template if it is not already: `Why This Exists` (with a
  Without/With comparison: without gorun you write `exec.Command` + Wait + kill on restart; with gorun one `RunProgram`),
  `Before You Start`, `Step-by-Step Guide` (one complete compilable example using `New`, `RunProgram`, `StopProgram`),
  `Documentation Map` (table linking `cleanup.go`, `RunProgram.go`, `StopProgram.go`). ≤ 150 lines.

## Delivery check

```bash
gotest
grep -rn "StopApp\|KillAllByName\|killAllUnix\|killAllWindows\|pgrep" --include=*.go .   # → empty
```

A PR that does not compile, or whose tests could not run, is a failed delivery. Environment:
`GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`. Commit after every stage; never run
`git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work. Stuck 30 minutes: open the PR
anyway, listing what fails.

## Stages

| # | Files | Done when |
|---|---|---|
| 1 | `cleanup.go`, `goRun.go` | builds; old functions gone |
| 2 | `cleanup_linux.go`, `cleanup_darwin.go`, `cleanup_windows.go`, `cleanup_other.go` | `GOOS=darwin go vet ./...` and `GOOS=windows go vet ./...` pass |
| 3 | `cleanup_test.go`, `cleanup_path_test.go` | the three tests pass on linux |
| 4 | `README.md` | template sections present |
