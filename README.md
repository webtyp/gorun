# gorun
<img src="docs/img/badges.svg">

gorun runs, monitors and stops external programs from Go, capturing their output and shutting them down gracefully.

## Why This Exists

Without gorun, managing external processes requires repetitive boilerplate: `exec.Command`, channel synchronization, handling stdout/stderr, and manually tracking PIDs to ensure proper termination.

With gorun, process management becomes declarative. A single `RunProgram` call handles execution and output forwarding, while `StopProgram` ensures cross-platform graceful termination without dangling processes.

## Before You Start

Install the package:

```sh
go get webtyp.com/gorun
```

## Step-by-Step Guide

Minimal example demonstrating execution and graceful termination:

```go
package main

import (
	"log"
	"time"

	"webtyp.com/gorun"
)

func main() {
	cfg := &gorun.Config{
		ExecProgramPath: "./my-server",
		WorkingDir:      "/abs/path/to/project", // optional
		ExitChan:        make(chan bool),
		KillAllOnStop:   true,
	}

	r := gorun.New(cfg)

	if err := r.RunProgram(); err != nil {
		log.Fatalf("Failed to run: %v", err)
	}

	// Wait for program to run
	time.Sleep(5 * time.Second)

	if err := r.StopProgram(); err != nil {
		log.Fatalf("Failed to stop: %v", err)
	}
}
```

With `KillAllOnStop`, `RunProgram` and `StopProgram` also kill orphans of the **same executable path** (left by a crashed parent). Two programs with the same file name in different folders never touch each other. `DisableGlobalCleanup: true` turns that off.

## Documentation Map

| Topic | File |
|---|---|
| Process execution and monitoring | `RunProgram.go` |
| Graceful cross-platform termination | `StopProgram.go` |
| Orphan process cleanup by path | `cleanup.go` |

## [Contributing](https://github.com/webtyp/cdvelop/blob/main/CONTRIBUTING.md)