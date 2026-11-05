//go:build windows

package gorun

import (
	"os/exec"
	"strconv"
	"strings"
)

const powershell = "powershell"

// psFilter lists the PIDs whose ExecutablePath equals %s (a single-quoted
// PowerShell literal: a quote inside the path is doubled).
const psFilter = "Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -eq '%s' } | ForEach-Object { $_.ProcessId }"

func listPIDsByExecutable(abs string) ([]int, error) {
	script := strings.Replace(psFilter, "%s", strings.ReplaceAll(abs, "'", "''"), 1)
	out, err := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(field); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
