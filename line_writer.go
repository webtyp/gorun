package gorun

import (
	"bytes"
	"sync"
)

// lineWriter is the io.Writer each child stream is copied into. It appends
// every byte to the SafeBuffer's raw buffer and forwards each COMPLETE line,
// without its terminator, as one Logger call. A partial line is held until its
// newline arrives or Flush is called at end of stream. Empty lines are not
// forwarded. One lineWriter per stream, so stdout and stderr never splice.
type lineWriter struct {
	sb      *SafeBuffer
	mu      sync.Mutex
	pending []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.sb.mutex.Lock()
	_, err := w.sb.buffer.Write(p)
	forwardTo := w.sb.forwardTo
	w.sb.mutex.Unlock()

	if err != nil {
		return 0, err
	}

	w.pending = append(w.pending, p...)

	for {
		idx := bytes.IndexByte(w.pending, '\n')
		if idx == -1 {
			break
		}
		line := w.pending[:idx]
		w.pending = w.pending[idx+1:]

		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}

		if len(line) > 0 && forwardTo != nil {
			forwardTo(string(line))
		}
	}

	if len(w.pending) == 0 {
		w.pending = w.pending[:0]
	}

	return len(p), nil
}

// Flush forwards a trailing line that never got its newline. Called once, when
// the stream has ended.
func (w *lineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.sb.mutex.RLock()
	forwardTo := w.sb.forwardTo
	w.sb.mutex.RUnlock()

	if len(w.pending) == 0 {
		return
	}

	line := w.pending
	w.pending = nil

	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}

	if len(line) > 0 && forwardTo != nil {
		forwardTo(string(line))
	}
}
