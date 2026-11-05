package gorun

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

func TestKillAllByPath_SameNameOtherDirSurvives(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process listing covered on linux/darwin")
	}

	longProgramPath := buildTestProgram(t, "long_program")

	dirA := t.TempDir()
	dirB := t.TempDir()
	serverA := filepath.Join(dirA, "server")
	serverB := filepath.Join(dirB, "server")

	if err := copyFile(longProgramPath, serverA); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(longProgramPath, serverB); err != nil {
		t.Fatal(err)
	}

	a := New(&Config{
		ExecProgramPath: serverA,
		KillAllOnStop:   true,
		ExitChan:        make(chan bool),
	})
	b := New(&Config{
		ExecProgramPath: serverB,
		KillAllOnStop:   true,
		ExitChan:        make(chan bool),
	})

	if err := a.RunProgram(); err != nil {
		t.Fatal(err)
	}
	if err := b.RunProgram(); err != nil {
		t.Fatal(err)
	}

	// Ensure they are running
	time.Sleep(100 * time.Millisecond)

	// Stop b. This should NOT kill a, because their paths are different.
	if err := b.StopProgram(); err != nil {
		t.Logf("b.StopProgram returned err: %v", err)
	}

	if !a.IsRunning() {
		t.Error("a is not running after b was stopped")
	}
	if a.Cmd != nil && a.Cmd.Process != nil {
		if err := a.Cmd.Process.Signal(syscall.Signal(0)); err != nil {
			t.Errorf("a process doesn't answer to signal 0: %v", err)
		}
	}

	if err := a.StopProgram(); err != nil {
		t.Logf("a.StopProgram returned err: %v", err)
	}
}

func TestKillAllByPath_KillsOrphanOfSamePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process listing covered on linux/darwin")
	}

	longProgramPath := buildTestProgram(t, "long_program")
	dirA := t.TempDir()
	serverA := filepath.Join(dirA, "server")
	if err := copyFile(longProgramPath, serverA); err != nil {
		t.Fatal(err)
	}

	orphan := exec.Command(serverA)
	if err := orphan.Start(); err != nil {
		t.Fatal(err)
	}

	orphanDone := make(chan error, 1)
	go func() {
		orphanDone <- orphan.Wait()
	}()

	time.Sleep(100 * time.Millisecond)

	a := New(&Config{
		ExecProgramPath: serverA,
		KillAllOnStop:   true,
		ExitChan:        make(chan bool),
	})

	if err := a.RunProgram(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-orphanDone:
		// success, orphan killed
	case <-time.After(3 * time.Second):
		t.Error("orphan process not killed within 3s")
		orphan.Process.Kill()
	}

	if !a.IsRunning() {
		t.Error("a is not running after killing orphan")
	}

	if err := a.StopProgram(); err != nil {
		t.Logf("a.StopProgram returned err: %v", err)
	}
}

func TestKillAllByPath_DisableGlobalCleanupSparesOrphan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process listing covered on linux/darwin")
	}

	longProgramPath := buildTestProgram(t, "long_program")
	dirA := t.TempDir()
	serverA := filepath.Join(dirA, "server")
	if err := copyFile(longProgramPath, serverA); err != nil {
		t.Fatal(err)
	}

	orphan := exec.Command(serverA)
	if err := orphan.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	a := New(&Config{
		ExecProgramPath:      serverA,
		KillAllOnStop:        true,
		DisableGlobalCleanup: true,
		ExitChan:             make(chan bool),
	})

	if err := a.RunProgram(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)

	if err := orphan.Process.Signal(syscall.Signal(0)); err != nil {
		t.Errorf("orphan should still be alive, but signal 0 failed: %v", err)
	}

	orphan.Process.Kill()
	orphan.Wait()

	if err := a.StopProgram(); err != nil {
		t.Logf("a.StopProgram returned err: %v", err)
	}
}
