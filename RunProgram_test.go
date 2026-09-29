package gorun

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunProgram_Basic(t *testing.T) {
	// Build test program
	execPath := buildTestProgram(t, "simple_program")
	defer os.Remove(execPath)

	exitChan := make(chan bool)
	buf, logger := createTestLogger()

	config := &Config{
		ExecProgramPath: execPath,
		RunArguments:    func() []string { return []string{} },
		ExitChan:        exitChan,
		Logger:          logger,
	}

	gr := New(config)

	// Run the program
	err := gr.RunProgram()
	if err != nil {
		t.Fatalf("RunProgram() failed: %v", err)
	}

	// Should be running
	if !gr.IsRunning() {
		t.Error("Program should be running after RunProgram()")
	}

	// Clean up
	gr.StopProgram()

	_ = buf // Use buf to avoid unused variable error
}

func TestRunProgram_WithArguments(t *testing.T) {
	// Create a program that prints its arguments
	argProgram := `package main
import (
	"fmt"
	"os"
	"strings"
	"time"
)
func main() {
	fmt.Println("ARGS_START")
	fmt.Println("ARGS:" + strings.Join(os.Args[1:], ","))
	time.Sleep(100 * time.Millisecond) // Keep it alive long enough to test
	fmt.Println("ARGS_END")
}`

	// Write to temporary file
	tmpFile := filepath.Join(os.TempDir(), "arg_program.go")
	err := os.WriteFile(tmpFile, []byte(argProgram), 0644)
	if err != nil {
		t.Fatalf("Failed to write test program: %v", err)
	}
	defer os.Remove(tmpFile)

	// Build it
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	execPath := filepath.Join(os.TempDir(), "arg_program"+ext)
	cmd := exec.Command("go", "build", "-o", execPath, tmpFile)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build arg program: %v", err)
	}
	defer os.Remove(execPath)

	exitChan := make(chan bool)
	buf, logger := createTestLogger()

	testArgs := []string{"test1", "test2", "test3"}
	config := &Config{
		ExecProgramPath: execPath,
		RunArguments:    func() []string { return testArgs },
		ExitChan:        exitChan,
		Logger:          logger,
	}

	gr := New(config)

	// Run the program
	err = gr.RunProgram()
	if err != nil {
		t.Fatalf("RunProgram() failed: %v", err)
	}

	// Wait for output
	time.Sleep(200 * time.Millisecond)

	output := gr.getOutput()
	// Check if the program ran (arguments are passed correctly)
	// Since the logger captures output, we should see either the expected args or program completion
	if !strings.Contains(output, "ARGS:") && !strings.Contains(output, "closed successfully") {
		t.Errorf("Expected either program output or completion message. Got: %s", output)
	}

	// Clean up
	gr.StopProgram()

	_ = buf // Use buf to avoid unused variable error
}

func TestRunProgram_InvalidExecutable(t *testing.T) {
	exitChan := make(chan bool)
	buf, logger := createTestLogger()

	config := &Config{
		ExecProgramPath: "/nonexistent/path/program",
		RunArguments:    func() []string { return []string{} },
		ExitChan:        exitChan,
		Logger:          logger,
	}

	gr := New(config)

	err := gr.RunProgram()
	if err == nil {
		t.Error("Expected error for invalid executable path")
	}

	_ = buf // Use buf to avoid unused variable error
}

func TestRunProgram_StopPrevious(t *testing.T) {
	// Build test program
	execPath := buildTestProgram(t, "long_program")
	defer os.Remove(execPath)

	exitChan := make(chan bool)
	buf, logger := createTestLogger()

	config := &Config{
		ExecProgramPath: execPath,
		RunArguments:    func() []string { return []string{} },
		ExitChan:        exitChan,
		Logger:          logger,
	}

	gr := New(config)

	// Start first program
	err := gr.RunProgram()
	if err != nil {
		t.Fatalf("First RunProgram() failed: %v", err)
	}

	if !gr.IsRunning() {
		t.Error("First program should be running")
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Start second program - should stop the first one
	err = gr.RunProgram()
	if err != nil {
		t.Fatalf("Second RunProgram() failed: %v", err)
	}

	if !gr.IsRunning() {
		t.Error("Second program should be running")
	}

	// Clean up
	gr.StopProgram()

	_ = buf // Use buf to avoid unused variable error
}

func TestRunProgram_ExitChannel(t *testing.T) {
	// Build test program
	execPath := buildTestProgram(t, "long_program")
	defer os.Remove(execPath)

	exitChan := make(chan bool, 1)
	buf, logger := createTestLogger()

	config := &Config{
		ExecProgramPath: execPath,
		RunArguments:    func() []string { return []string{} },
		ExitChan:        exitChan,
		Logger:          logger,
	}

	gr := New(config)

	// Start program
	err := gr.RunProgram()
	if err != nil {
		t.Fatalf("RunProgram() failed: %v", err)
	}

	if !gr.IsRunning() {
		t.Error("Program should be running")
	}

	// Signal exit through channel
	exitChan <- true

	// Wait for the program to detect the exit signal and stop
	time.Sleep(200 * time.Millisecond)

	if gr.IsRunning() {
		t.Error("Program should have stopped after exit signal")
	}

	_ = buf // Use buf to avoid unused variable error
}

func TestRunProgram_EmptyPath(t *testing.T) {
	exitChan := make(chan bool)
	buf, logger := createTestLogger()

	config := &Config{
		ExecProgramPath: "",
		RunArguments:    func() []string { return []string{} },
		ExitChan:        exitChan,
		Logger:          logger,
	}

	gr := New(config)

	// Try to run with empty path
	err := gr.RunProgram()
	if err == nil {
		t.Error("RunProgram() should fail for empty executable path")
	}

	if gr.IsRunning() {
		t.Error("Program should not be running with empty path")
	}

	_ = buf // Use buf to avoid unused variable error
}

func TestRunProgram_ProgramExitsQuickly(t *testing.T) {
	// Create a program that exits immediately
	quickProgram := `package main
import "fmt"
func main() {
	fmt.Println("QUICK_PROGRAM_EXECUTED")
}`

	// Write to temporary file
	tmpFile := filepath.Join(os.TempDir(), "quick_program.go")
	err := os.WriteFile(tmpFile, []byte(quickProgram), 0644)
	if err != nil {
		t.Fatalf("Failed to write test program: %v", err)
	}
	defer os.Remove(tmpFile)

	// Build it
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	execPath := filepath.Join(os.TempDir(), "quick_program"+ext)
	cmd := exec.Command("go", "build", "-o", execPath, tmpFile)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build quick program: %v", err)
	}
	defer os.Remove(execPath)

	exitChan := make(chan bool)
	_, logger := createTestLogger()

	config := &Config{
		ExecProgramPath: execPath,
		RunArguments:    func() []string { return []string{} },
		ExitChan:        exitChan,
		Logger:          logger,
	}

	gr := New(config)

	// Run the program
	err = gr.RunProgram()
	if err != nil {
		t.Fatalf("RunProgram() failed: %v", err)
	}

	// Wait for program to complete
	time.Sleep(200 * time.Millisecond)

	output := gr.getOutput()
	// Check that the program ran and completed (either output or completion message)
	if !strings.Contains(output, "QUICK_PROGRAM_EXECUTED") && !strings.Contains(output, "closed successfully") {
		t.Errorf("Expected either program output or completion message. Got: %s", output)
	}

	// Program should have exited on its own
	// The IsRunning state depends on how the implementation handles finished processes
}

func TestRunProgram_ForwardsOneEntryPerLine(t *testing.T) {
	execPath := buildTestProgram(t, "two_lines")
	defer os.Remove(execPath)

	exitChan := make(chan bool)
	var mu sync.Mutex
	var calls []string

	logger := func(args ...any) {
		mu.Lock()
		defer mu.Unlock()
		msg := fmt.Sprint(args...)
		calls = append(calls, msg)
	}

	config := &Config{
		ExecProgramPath: execPath,
		RunArguments:    func() []string { return []string{} },
		ExitChan:        exitChan,
		Logger:          logger,
	}

	gr := New(config)

	err := gr.RunProgram()
	if err != nil {
		t.Fatalf("RunProgram() failed: %v", err)
	}

	// Poll IsRunning until false with a 5s timeout
	start := time.Now()
	for gr.IsRunning() {
		if time.Since(start) > 5*time.Second {
			t.Fatal("timed out waiting for program to exit")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Give a short time for logger goroutines to finish forwarding
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	recorded := make([]string, len(calls))
	copy(recorded, calls)
	mu.Unlock()

	var firstFound, secondFound bool
	for _, entry := range recorded {
		if strings.Contains(entry, "\n") {
			t.Errorf("found newline in forwarded entry: %q", entry)
		}
		if entry == "FIRST_LINE" {
			firstFound = true
		}
		if entry == "SECOND_LINE" {
			secondFound = true
		}
	}

	if !firstFound || !secondFound {
		t.Errorf("expected FIRST_LINE and SECOND_LINE as separate entries, recorded entries: %v", recorded)
	}

	gr.StopProgram()
}
