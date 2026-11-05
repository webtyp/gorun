//go:build linux

package gorun

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// procDir and deletedSuffix are Linux /proc facts: /proc/<pid>/exe links to the
// executable, and reads "<path> (deleted)" once the file was rebuilt under it.
const (
	procDir       = "/proc"
	procExe       = "exe"
	deletedSuffix = " (deleted)"
)

func listPIDsByExecutable(abs string) ([]int, error) {
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // not a process directory
		}
		target, err := os.Readlink(filepath.Join(procDir, e.Name(), procExe))
		if err != nil {
			continue // another user's process, or it just exited
		}
		if strings.TrimSuffix(target, deletedSuffix) == abs {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
