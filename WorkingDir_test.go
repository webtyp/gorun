package gorun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkingDir(t *testing.T) {
	// Create a temporary test structure
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Error creating subdirectory: %v", err)
	}

	exitChan := make(chan bool, 1)

	// Use 'pwd' command to test working directory
	config := &Config{
		ExecProgramPath: "pwd",
		ExitChan:        exitChan,
		Logger:          nil,    // use internal SafeBuffer and read via GetOutput()
		WorkingDir:      subDir, // Set working directory to subdirectory
	}

	gorun := New(config)

	t.Run("WorkingDir is set correctly", func(t *testing.T) {
		err := gorun.RunProgram()
		if err != nil {
			t.Fatalf("Error running program: %v", err)
		}

		outputStr := waitForOutput(t, gorun, 2*time.Second)

		// Stop the program
		gorun.StopProgram()

		// Check if the output contains the expected working directory (thread-safe)
		if !strings.Contains(outputStr, subDir) {
			t.Errorf("Expected working directory %s in output, got: %s", subDir, outputStr)
		}
	})
}

func TestWorkingDirNotSet(t *testing.T) {
	// Test that when WorkingDir is not set, it uses the default working directory
	exitChan := make(chan bool, 1)

	config := &Config{
		ExecProgramPath: "pwd",
		ExitChan:        exitChan,
		Logger:          nil,
		// WorkingDir not set - should use current directory
	}

	gorun := New(config)

	t.Run("WorkingDir not set uses current directory", func(t *testing.T) {
		err := gorun.RunProgram()
		if err != nil {
			t.Fatalf("Error running program: %v", err)
		}

		outputStr := waitForOutput(t, gorun, 2*time.Second)

		// Stop the program
		gorun.StopProgram()

		// Get current working directory
		currentDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Error getting current directory: %v", err)
		}

		// Check if the output contains the current working directory (thread-safe)
		if !strings.Contains(outputStr, currentDir) {
			t.Errorf("Expected current directory %s in output, got: %s", currentDir, outputStr)
		}
	})
}

// waitForOutput polls getOutput() until it is non-empty or timeout elapses,
// instead of a fixed sleep — the short-lived "pwd" process this file tests
// can take longer than a fixed delay to exit and have its output copied
// under system load, which made these tests flaky.
func waitForOutput(t *testing.T, gr *GoRun, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if out := gr.getOutput(); out != "" {
			return out
		}
		time.Sleep(10 * time.Millisecond)
	}
	return gr.getOutput()
}
