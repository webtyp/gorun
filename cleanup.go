package gorun

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

// StopProgramAndCleanup stops the current program and optionally kills every other
// process of the same executable path (orphans of a crashed parent)
func (h *GoRun) StopProgramAndCleanup(killAll bool) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return h.stopProgramAndCleanupUnsafe(killAll)
}

// stopProgramAndCleanupUnsafe is the unsafe version that doesn't acquire mutex
func (h *GoRun) stopProgramAndCleanupUnsafe(killAll bool) error {
	// First stop our specific process
	err := h.stopProgramUnsafe()

	// If requested, also kill other instances of this exact executable
	// (orphans of a parent that died without stopping its child).
	if killAll && h.ExecProgramPath != "" && !h.DisableGlobalCleanup {
		if cleanupErr := killAllByPath(h.ExecProgramPath); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to cleanup other instances of %s: %v\n", h.ExecProgramPath, cleanupErr)
		}
	}

	return err
}
