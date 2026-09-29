package gorun

import (
	"bytes"
	"sync"
)

// SafeBuffer provides thread-safe operations on a bytes.Buffer
// and optionally forwards writes to a function logger
type SafeBuffer struct {
	buffer    *bytes.Buffer
	mutex     sync.RWMutex
	forwardTo func(message ...any) // Optional function logger to forward data to
}

// NewSafeBuffer creates a new thread-safe buffer
func NewSafeBuffer() *SafeBuffer {
	return &SafeBuffer{
		buffer: &bytes.Buffer{},
	}
}

// NewSafeBufferWithForward creates a new thread-safe buffer that forwards writes
func NewSafeBufferWithForward(forward func(message ...any)) *SafeBuffer {
	return &SafeBuffer{
		buffer:    &bytes.Buffer{},
		forwardTo: forward,
	}
}

// Write writes data to the buffer and optionally forwards to another writer
func (sb *SafeBuffer) Write(p []byte) (n int, err error) {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	// Write to internal buffer first
	n, err = sb.buffer.Write(p)
	if err != nil {
		return n, err
	}

	// Forward to function logger if configured
	if sb.forwardTo != nil {
		parts := bytes.Split(p, []byte("\n"))
		for _, part := range parts {
			if len(part) > 0 && part[len(part)-1] == '\r' {
				part = part[:len(part)-1]
			}
			if len(part) > 0 {
				sb.forwardTo(string(part))
			}
		}
	}

	return n, err
}

// String returns the contents of the buffer as a string in a thread-safe manner
func (sb *SafeBuffer) String() string {
	sb.mutex.RLock()
	defer sb.mutex.RUnlock()
	return sb.buffer.String()
}

// lineWriter returns a new per-stream writer bound to this buffer.
func (sb *SafeBuffer) lineWriter() *lineWriter {
	return &lineWriter{sb: sb}
}

// Reset resets the buffer in a thread-safe manner
func (sb *SafeBuffer) Reset() {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()
	sb.buffer.Reset()
}

// Len returns the length of the buffer in a thread-safe manner
func (sb *SafeBuffer) Len() int {
	sb.mutex.RLock()
	defer sb.mutex.RUnlock()
	return sb.buffer.Len()
}
