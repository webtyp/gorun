//go:build darwin

package gorun

import (
	"os/exec"
	"strconv"
	"strings"
)

const psCommand = "ps"

var psArgs = []string{"-axo", "pid=,comm="}

func listPIDsByExecutable(abs string) ([]int, error) {
	out, err := exec.Command(psCommand, psArgs...).Output()
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		sep := strings.IndexByte(line, ' ')
		if sep < 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:sep])
		if err != nil {
			continue
		}
		if strings.TrimSpace(line[sep+1:]) == abs {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
