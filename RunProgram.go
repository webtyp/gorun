package gorun

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

func (h *GoRun) RunProgram() error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Always stop any previous running program first
	// Use cleanup if KillAllOnStop is enabled
	if h.KillAllOnStop {
		if err := h.stopProgramAndCleanupUnsafe(true); err != nil {
			h.safeBuffer.Write([]byte(fmt.Sprintf("Warning: Error stopping previous programs: %v\n", err)))
		}
	} else {
		if err := h.stopProgramUnsafe(); err != nil {
			h.safeBuffer.Write([]byte(fmt.Sprintf("Warning: Error stopping previous program: %v\n", err)))
		}
	}

	runArgs := []string{}

	if h.RunArguments != nil {
		runArgs = h.RunArguments()
	}

	h.Cmd = exec.Command(h.ExecProgramPath, runArgs...)
	h.hasWaited = false // Reset wait flag for new process

	// DEBUG: Log the exact run command being executed
	// fmt.Fprintf(h.safeBuffer, "[GORUN DEBUG] Starting: %s %v\n", h.ExecProgramPath, runArgs)
	// fmt.Fprintf(h.safeBuffer, "[GORUN DEBUG] Working dir: %s\n", h.WorkingDir)

	// Set working directory if specified
	if h.WorkingDir != "" {
		h.Cmd.Dir = h.WorkingDir
	}

	// Forward the project's .env into the child, without overriding
	// anything already exported in this process's own environment — same
	// priority order webtyp.com/env already uses (process env wins).
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

	stderr, err := h.Cmd.StderrPipe()
	if err != nil {
		return err
	}

	stdout, err := h.Cmd.StdoutPipe()
	if err != nil {
		return err
	}

	err = h.Cmd.Start()
	if err != nil {
		// DEBUG: Log start failure details
		// fmt.Fprintf(h.safeBuffer, "[GORUN DEBUG] Failed to start process: %v\n", err)
		// Clean up the failed command to prevent issues in subsequent operations
		h.Cmd = nil
		h.isRunning = false
		h.hasWaited = false
		return err
	}

	// DEBUG: Log successful start
	// fmt.Fprintf(h.safeBuffer, "[GORUN DEBUG] Process started successfully with PID: %d\n", h.Cmd.Process.Pid)

	h.isRunning = true

	var once sync.Once
	done := make(chan struct{})

	// Create local references for goroutines to avoid race conditions
	currentCmd := h.Cmd

	go io.Copy(h.safeBuffer, stderr)
	go io.Copy(h.safeBuffer, stdout)

	go func() {
		select {
		case <-h.ExitChan:
			// h.Print("Received exit signal, stopping application...")
			h.StopProgram()
			once.Do(func() { close(done) })
		case <-done:
			// finish goroutine
		}
	}()

	go func() {
		err := currentCmd.Wait()
		h.mutex.Lock()
		h.isRunning = false
		h.hasWaited = true // Mark that Wait() has been called
		h.mutex.Unlock()

		if err != nil {
			// Check if the error is due to a signal termination (normal shutdown)
			errMsg := err.Error()
			if !strings.Contains(errMsg, "signal: terminated") &&
				!strings.Contains(errMsg, "signal: killed") &&
				!strings.Contains(errMsg, "signal: interrupt") &&
				!strings.Contains(errMsg, "waitid: no child processes") {
				// This is an actual error, not a normal signal termination
				h.safeBuffer.Write([]byte(fmt.Sprintf("App: %v closed with error: %v\n", h.ExecProgramPath, err)))
			}
			// No log for normal signal terminations or known benign waitid error
		}
		// No log for clean exits either
		once.Do(func() { close(done) })
	}()

	return nil
}
