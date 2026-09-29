---
PLAN: "fix: forward child process output one line at a time, per stream"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 398667440618462986
PR: https://github.com/webtyp/gorun/pull/4
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — the Logger receives one message per output line

## Why

`gorun` starts a child process and forwards its output to `Config.Logger`.
Today both pipes are copied straight into the same `SafeBuffer`:

```go
// RunProgram.go
go io.Copy(h.safeBuffer, stderr)
go io.Copy(h.safeBuffer, stdout)
```

and `SafeBuffer.Write` forwards each raw chunk it receives:

```go
if sb.forwardTo != nil {
	sb.forwardTo(string(p))
}
```

A chunk is whatever one `read` returned, not a line. The developer TUI that
consumes this Logger (`webtyp.com/server` → `webtyp.com/app`) therefore shows:

1. **A blank line after almost every message** — the chunk's trailing `"\n"`
   is forwarded as part of the message.
2. **Two messages fused into one** — when the child writes two lines quickly
   they arrive in one chunk, e.g. this single log entry:

   ```
   app: SECURITY EVENT — type: 15 userID: 1789255672822246364 resource:  IP:
   auth: DEV_AUTOLOGIN opened a session for user 1789255672822246364
   ```

3. Because stdout and stderr share one destination, a partial line from one
   stream can be glued to a line from the other.

## What to change

The Logger must receive **exactly one call per complete output line**, without
the line terminator, and **empty lines are not forwarded**. Each stream
(stdout, stderr) is split into lines **independently**. The raw buffer read by
`GetOutput()` keeps receiving every byte unchanged, so `GetOutput()` output does
not change.

## Design gate

- **Prior art.** `bufio.Scanner` with `ScanLines` — the standard way to turn a
  byte stream into lines; `log.Logger` emits one entry per call; `exec.Cmd`
  users routinely wrap pipes in a line splitter. Line-per-entry is what every
  log viewer expects.
- **Novice-name test.** No new exported names. The exported API
  (`SafeBuffer`, `NewSafeBuffer`, `NewSafeBufferWithForward`, `Config.Logger`)
  keeps its signatures; only what the forward function receives changes: whole
  lines instead of raw chunks.
- **Complexity ledger.** +1 unexported type (`lineWriter`), +1 file. +0
  exported concepts.
- **Where it belongs.** Here: `gorun` is the only place that sees the child's
  raw pipes. Consumers (`webtyp/server`, `webtyp/app`) cannot re-split lines
  after two streams have already been merged.
- **What it deletes.** The two `go io.Copy(h.safeBuffer, …)` lines, and the
  raw-chunk forwarding (`sb.forwardTo(string(p))`) in `SafeBuffer.Write`.

## Repo rules

- This is backend tooling: it legitimately uses the standard library
  (`bytes`, `io`, `strings`, `sync`). Do NOT replace those imports.
- Tests in this repo live next to the code as `*_test.go` in `package gorun`
  (e.g. `RunProgram_test.go`). Follow that layout. Test programs live in
  `testdata/*.go` and are built with the existing helper
  `buildTestProgram(t, name)` from `gorun_test.go`.

## Stage 1 — `line_writer.go` (new file)

```go
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

func (w *lineWriter) Write(p []byte) (int, error) { … }

// Flush forwards a trailing line that never got its newline. Called once, when
// the stream has ended.
func (w *lineWriter) Flush() { … }
```

Behavior, exactly:

- `Write(p)`:
  1. Write `p` to the SafeBuffer's raw buffer under the SafeBuffer's mutex
     (same as `SafeBuffer.Write` does today for the buffer part).
  2. Append `p` to `pending`. While `pending` contains `'\n'`: take the bytes
     before it as `line`, remove `line` + `'\n'` from `pending`, trim one
     trailing `'\r'` from `line` (Windows `\r\n`), and if `line` is not empty
     after that, call the forward function with `string(line)`.
  3. Return `len(p), nil`.
- `Flush()`: if `pending` is non-empty after trimming one trailing `'\r'`,
  forward it and reset `pending`.
- If the SafeBuffer has no forward function (`forwardTo == nil`), still write
  to the raw buffer; skip forwarding.

Add to `safe_buffer.go`:

```go
// lineWriter returns a new per-stream writer bound to this buffer.
func (sb *SafeBuffer) lineWriter() *lineWriter { return &lineWriter{sb: sb} }
```

## Stage 2 — `safe_buffer.go`: `Write` forwards lines too

`SafeBuffer.Write` stays exported with the same signature (it is used directly
for gorun's own messages, e.g. `"App: %v closed with error: %v\n"`). Change its
forwarding so that, instead of `sb.forwardTo(string(p))`, it splits `p` on
`'\n'`, trims one trailing `'\r'` from each part, and forwards each non-empty
part as its own call. (These direct writes are always whole lines, so no
pending state is needed here.)

Acceptance: `grep -n "forwardTo(string(p))" safe_buffer.go` → empty.

## Stage 3 — `RunProgram.go`: one lineWriter per stream

Replace:

```go
go io.Copy(h.safeBuffer, stderr)
go io.Copy(h.safeBuffer, stdout)
```

with:

```go
for _, stream := range []io.Reader{stderr, stdout} {
	lw := h.safeBuffer.lineWriter()
	go func(r io.Reader) {
		io.Copy(lw, r)
		lw.Flush()
	}(stream)
}
```

Acceptance: `grep -n "io.Copy(h.safeBuffer" RunProgram.go` → empty.

## Stage 4 — tests

### `line_writer_test.go` (new, `package gorun`)

Use a SafeBuffer created with `NewSafeBufferWithForward(func(m ...any) { … })`
that records each call's joined text into a slice.

| Test | Writes | Forwarded calls expected |
|---|---|---|
| `TestLineWriter_OneCallPerLine` | `"a\nb\n"` in one Write | `["a", "b"]` |
| `TestLineWriter_NoBlankEntries` | `"a\n\n\nb\n"` | `["a", "b"]` |
| `TestLineWriter_PartialLineHeldUntilNewline` | `"hel"`, then `"lo\nwor"`, then `"ld\n"` | `["hello", "world"]` |
| `TestLineWriter_CRLF` | `"a\r\nb\r\n"` | `["a", "b"]` |
| `TestLineWriter_FlushForwardsTail` | `"tail"` then `Flush()` | `["tail"]` |
| `TestLineWriter_RawBufferUnchanged` | `"a\n\nb"` then `Flush()` | `sb.String() == "a\n\nb"` |
| `TestSafeBufferWrite_SplitsLines` | `sb.Write([]byte("x\ny\n"))` | `["x", "y"]` |

### `RunProgram_test.go` — end to end

Add `testdata/two_lines.go`:

```go
package main

import "os"

func main() {
	// One write, two lines: arrives as a single chunk on the pipe.
	os.Stdout.Write([]byte("FIRST_LINE\nSECOND_LINE\n"))
}
```

Add `TestRunProgram_ForwardsOneEntryPerLine`: build `two_lines` with
`buildTestProgram`, run it with a Logger that records calls (same pattern as
`createTestLogger`, but recording each call separately), wait until the program
exits (poll `IsRunning()` with a 5 s timeout, as other tests in this file do),
then assert the recorded calls contain `"FIRST_LINE"` and `"SECOND_LINE"` as
**two separate entries**, and that no entry contains `"\n"`.

## Stage 5 — verify

- `go test ./...` passes (all existing tests unchanged).
- Both greps from stages 2 and 3 are empty.

## Stages

| Stage | Files | Done when |
|---|---|---|
| 1 | `line_writer.go` (new), `safe_buffer.go` | `lineWriter` + `(*SafeBuffer).lineWriter()` implemented as specified |
| 2 | `safe_buffer.go` | `Write` forwards one call per non-empty line |
| 3 | `RunProgram.go` | one `lineWriter` per stream, flushed at end of stream |
| 4 | `line_writer_test.go` (new), `RunProgram_test.go`, `testdata/two_lines.go` (new) | all listed tests pass |
| 5 | — | full suite green, greps empty |
