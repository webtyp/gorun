---
PLAN: "feat: forward a project's .env file into the child process environment"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `Config.EnvFile`: make a project's `.env` visible to the process `gorun` starts

## Part of a multi-repo wave

This is module 1 of `DOTENV_VISIBILITY_MASTER_PLAN.md` (orchestrator:
`webtyp.com/docs`, `docs/DOTENV_VISIBILITY_MASTER_PLAN.md`). Nothing else
must publish before this one — it has no dependencies. `webtyp/server`
Stage B depends on the tag this plan produces.

## Why

`gorun.Config.WorkingDir` sets the child process's CWD to whatever the
caller passes — for `webtyp/server`'s external-mode strategy, that is the
project's build output directory, not its root. Any library the child links
that reads config via a relative `.env` lookup (`webtyp.com/env`) silently
finds nothing, because the project's real `.env` lives next to `go.mod`, not
in the output directory. The child process never inherits it either: nothing
upstream of `gorun` loads the project's `.env` into its own OS environment
before spawning.

The fix belongs here, not in `webtyp/server` alone and not in `webtyp/env`:
`gorun` already owns "how a child process gets started" (`WorkingDir`,
`RunArguments`, `ExitChan` are all here). Environment is the same concern.
Making the child's environment explicit and CWD-independent is strictly
better than trying to make `.env` lookups CWD-aware from inside the child —
one caller states which file to forward, once, instead of every downstream
consumer having to guess its own working directory.

**Anti-footgun:** `gorun` is backend-only developer tooling (it starts and
manages OS processes on the machine running `webtyp dev`), never compiled to
WASM. It legitimately uses `os`, `strings`, `fmt` from the standard library.
Do NOT "fix" those imports to `webtyp.com/fmt` — that rule applies to
WASM-shared code, not this package.

## What to change

### 1. `Config` — new field, `gorun/goRun.go`

```go
type Config struct {
	ExecProgramPath      string          // eg: "server/main.exe"
	RunArguments         func() []string // eg: []string{"dev"}
	ExitChan             chan bool
	Logger               func(message ...any)
	KillAllOnStop        bool   // If true, kills all instances of the executable when stopping
	DisableGlobalCleanup bool   // If true, disables global cleanup (pgrep -f) even if KillAllOnStop is true
	WorkingDir           string // eg: "/path/to/working/dir"
	EnvFile              string // eg: "/path/to/project/.env" — parsed and merged into the child's environment, lower priority than the parent process's own environment. Empty ("") disables this feature entirely.
}
```

### 2. New file `gorun/envfile.go`

```go
package gorun

import (
	"os"
	"strings"
)

// parseEnvFile reads a KEY=VALUE .env file and returns its pairs as
// "KEY=VALUE" strings, ready to append to an exec.Cmd.Env slice. Lines that
// are blank, start with "#", or do not contain "=" are skipped. A value
// wrapped in double quotes has them stripped. A missing file is not an
// error — the wave that added EnvFile is optional per-project, and no error
// here becomes a loud diagnostic surfaced through Logger to keep the
// caller in charge of "this file must exist" decisions, if any.
func parseEnvFile(path string, logger func(message ...any)) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) && logger != nil {
			logger("gorun: EnvFile", path, "could not be read:", err)
		}
		return nil
	}

	var pairs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		pairs = append(pairs, key+"="+value)
	}
	return pairs
}
```

### 3. `RunProgram.go` — merge into `Cmd.Env`, right after `Cmd.Dir` is set

In `gorun/RunProgram.go`, immediately after this existing block (around line
41-43):

```go
	// Set working directory if specified
	if h.WorkingDir != "" {
		h.Cmd.Dir = h.WorkingDir
	}
```

add:

```go
	// Forward the project's .env into the child, without overriding
	// anything already exported in this process's own environment —
	// same priority order webtyp.com/env already uses (os env wins).
	if h.EnvFile != "" {
		env := os.Environ()
		existing := make(map[string]bool, len(env))
		for _, kv := range env {
			if k, _, ok := strings.Cut(kv, "="); ok {
				existing[k] = true
			}
		}
		for _, kv := range parseEnvFile(h.EnvFile, h.Logger) {
			k, _, _ := strings.Cut(kv, "=")
			if !existing[k] {
				env = append(env, kv)
			}
		}
		h.Cmd.Env = env
	}
```

This requires adding `"os"` to `RunProgram.go`'s import block — `strings` is
already imported there (used for the `signal: terminated` checks further
down), do not add it twice.

## Tests (`gorun/envfile_test.go`, new file)

- `parseEnvFile` on a file with `KEY=value`, a comment line, a blank line,
  and `QUOTED="a b"` → returns `["KEY=value", "QUOTED=a b"]` (order
  preserved, comment and blank line absent).
- `parseEnvFile` on a nonexistent path → returns `nil`, and the passed
  logger is never called (no false-positive noise for the common "no .env"
  case).
- `parseEnvFile` on a path that exists but is a directory (or otherwise
  unreadable) → returns `nil` and the passed logger IS called once with a
  message containing the path.
- A `RunProgram` integration test (mirror the existing `WorkingDir` test in
  this package, if any, for style): set `EnvFile` to a temp file containing
  `GORUN_TEST_KEY=from_file`, run a tiny helper program that prints
  `os.Getenv("GORUN_TEST_KEY")`, assert the captured output is
  `from_file`.
- Same integration test, but with `GORUN_TEST_KEY` already set via
  `t.Setenv` in the test process before calling `RunProgram` → assert the
  child sees the `t.Setenv` value, not the file's (process env wins).

## What this does NOT change

- `WorkingDir` behavior is untouched — this is additive, gated entirely by
  whether the caller sets `EnvFile`.
- No existing caller of `gorun.Config` breaks: `EnvFile` defaults to `""`,
  and the merge block only runs when it is non-empty.

## Stages

| Stage | Files | Done when |
|---|---|---|
| 1 | `gorun/goRun.go` | `EnvFile` field added with doc comment |
| 2 | `gorun/envfile.go` (new) | `parseEnvFile` implemented as specified |
| 3 | `gorun/RunProgram.go` | merge block added after `Cmd.Dir` assignment |
| 4 | `gorun/envfile_test.go` (new) | all four tests above pass |
