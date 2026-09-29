package gorun

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func createRecordingLogger() (*SafeBuffer, *[]string, func(...any)) {
	var mu sync.Mutex
	var calls []string

	forward := func(args ...any) {
		mu.Lock()
		defer mu.Unlock()
		msg := fmt.Sprint(args...)
		calls = append(calls, msg)
	}

	sb := NewSafeBufferWithForward(forward)
	return sb, &calls, forward
}

func getCalls(calls *[]string) []string {
	res := make([]string, len(*calls))
	copy(res, *calls)
	return res
}

func TestLineWriter_OneCallPerLine(t *testing.T) {
	sb, calls, _ := createRecordingLogger()
	lw := sb.lineWriter()

	_, err := lw.Write([]byte("a\nb\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"a", "b"}
	if got := getCalls(calls); !reflect.DeepEqual(got, expected) {
		t.Errorf("got %v, want %v", got, expected)
	}
}

func TestLineWriter_NoBlankEntries(t *testing.T) {
	sb, calls, _ := createRecordingLogger()
	lw := sb.lineWriter()

	_, err := lw.Write([]byte("a\n\n\nb\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"a", "b"}
	if got := getCalls(calls); !reflect.DeepEqual(got, expected) {
		t.Errorf("got %v, want %v", got, expected)
	}
}

func TestLineWriter_PartialLineHeldUntilNewline(t *testing.T) {
	sb, calls, _ := createRecordingLogger()
	lw := sb.lineWriter()

	lw.Write([]byte("hel"))
	if got := getCalls(calls); len(got) != 0 {
		t.Errorf("expected 0 calls for partial line, got %v", got)
	}

	lw.Write([]byte("lo\nwor"))
	if got := getCalls(calls); !reflect.DeepEqual(got, []string{"hello"}) {
		t.Errorf("got %v, want [hello]", got)
	}

	lw.Write([]byte("ld\n"))
	expected := []string{"hello", "world"}
	if got := getCalls(calls); !reflect.DeepEqual(got, expected) {
		t.Errorf("got %v, want %v", got, expected)
	}
}

func TestLineWriter_CRLF(t *testing.T) {
	sb, calls, _ := createRecordingLogger()
	lw := sb.lineWriter()

	_, err := lw.Write([]byte("a\r\nb\r\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"a", "b"}
	if got := getCalls(calls); !reflect.DeepEqual(got, expected) {
		t.Errorf("got %v, want %v", got, expected)
	}
}

func TestLineWriter_FlushForwardsTail(t *testing.T) {
	sb, calls, _ := createRecordingLogger()
	lw := sb.lineWriter()

	lw.Write([]byte("tail"))
	if got := getCalls(calls); len(got) != 0 {
		t.Errorf("expected 0 calls before flush, got %v", got)
	}

	lw.Flush()
	expected := []string{"tail"}
	if got := getCalls(calls); !reflect.DeepEqual(got, expected) {
		t.Errorf("got %v, want %v", got, expected)
	}
}

func TestLineWriter_RawBufferUnchanged(t *testing.T) {
	sb, _, _ := createRecordingLogger()
	lw := sb.lineWriter()

	lw.Write([]byte("a\n\nb"))
	lw.Flush()

	expectedStr := "a\n\nb"
	if got := sb.String(); got != expectedStr {
		t.Errorf("got raw buffer %q, want %q", got, expectedStr)
	}
}

func TestSafeBufferWrite_SplitsLines(t *testing.T) {
	sb, calls, _ := createRecordingLogger()

	_, err := sb.Write([]byte("x\ny\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"x", "y"}
	if got := getCalls(calls); !reflect.DeepEqual(got, expected) {
		t.Errorf("got %v, want %v", got, expected)
	}
}
