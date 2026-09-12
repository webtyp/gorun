package gorun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseEnvFile_ParsesPairsSkipsCommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "KEY=value\n# a comment\n\nQUOTED=\"a b\"\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := parseEnvFile(path, nil)
	want := []string{"KEY=value", "QUOTED=a b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pair %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseEnvFile_MissingFileReturnsNilNoLog(t *testing.T) {
	var logged bool
	logger := func(message ...any) { logged = true }

	got := parseEnvFile(filepath.Join(t.TempDir(), "does-not-exist.env"), logger)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if logged {
		t.Error("logger should not be called for a simply-missing file")
	}
}

func TestParseEnvFile_UnreadablePathLogsAndReturnsNil(t *testing.T) {
	// A directory is not a valid .env file — os.ReadFile fails with a
	// non-NotExist error, which must be surfaced through the logger.
	dir := t.TempDir()
	var messages []any
	logger := func(message ...any) { messages = append(messages, message...) }

	got := parseEnvFile(dir, logger)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if len(messages) == 0 {
		t.Fatal("logger should be called for a read error other than NotExist")
	}
	found := false
	for _, m := range messages {
		if s, ok := m.(string); ok && strings.Contains(s, dir) {
			found = true
		}
	}
	if !found {
		t.Errorf("logged messages %v should name the path %q", messages, dir)
	}
}

func TestRunProgram_EnvFileVisibleToChild(t *testing.T) {
	execPath := buildTestProgram(t, "envprint")
	defer os.Remove(execPath)

	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("GORUN_TEST_KEY=from_file\n"), 0644); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("GORUN_TEST_KEY")

	exitChan := make(chan bool)
	config := &Config{
		ExecProgramPath: execPath,
		ExitChan:        exitChan,
		EnvFile:         envFile,
	}
	gr := New(config)

	if err := gr.RunProgram(); err != nil {
		t.Fatalf("RunProgram() failed: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	gr.StopProgram()

	if got := gr.getOutput(); !strings.Contains(got, "GORUN_TEST_KEY=from_file") {
		t.Errorf("output %q should contain the value from EnvFile", got)
	}
}

func TestRunProgram_ProcessEnvWinsOverEnvFile(t *testing.T) {
	execPath := buildTestProgram(t, "envprint")
	defer os.Remove(execPath)

	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("GORUN_TEST_KEY=from_file\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GORUN_TEST_KEY", "from_process")

	exitChan := make(chan bool)
	config := &Config{
		ExecProgramPath: execPath,
		ExitChan:        exitChan,
		EnvFile:         envFile,
	}
	gr := New(config)

	if err := gr.RunProgram(); err != nil {
		t.Fatalf("RunProgram() failed: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	gr.StopProgram()

	if got := gr.getOutput(); !strings.Contains(got, "GORUN_TEST_KEY=from_process") {
		t.Errorf("output %q should contain the process env value, not the .env one", got)
	}
}
