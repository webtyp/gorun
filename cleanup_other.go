//go:build !linux && !darwin && !windows

package gorun

import (
	"fmt"
	"runtime"
)

const errUnsupportedOS = "gorun: cannot list processes by executable path on %s"

func listPIDsByExecutable(abs string) ([]int, error) {
	return nil, fmt.Errorf(errUnsupportedOS, runtime.GOOS)
}
