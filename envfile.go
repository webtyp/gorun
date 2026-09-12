package gorun

import (
	"os"
	"strings"
)

// parseEnvFile reads a KEY=VALUE .env file and returns its pairs as
// "KEY=VALUE" strings, ready to append to an exec.Cmd.Env slice. Lines that
// are blank, start with "#", or do not contain "=" are skipped. A value
// wrapped in double quotes has them stripped. A missing file is not an
// error — EnvFile is optional per-project — so nothing is logged for that
// case; any other read error is reported through logger so it stays a loud
// diagnostic instead of a silently empty environment.
func parseEnvFile(path string, logger func(message ...any)) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) && logger != nil {
			logger("gorun: EnvFile", path, "could not be read:", err)
		}
		return nil
	}

	var pairs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		pairs = append(pairs, key+"="+value)
	}
	return pairs
}
