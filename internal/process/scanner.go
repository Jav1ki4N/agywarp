package process

import (
	"context"
	"strings"
)

// Scanner defines the interface for discovering active processes
type Scanner interface {
	Scan(ctx context.Context) ([]RunningProcess, error)
}

// CountRunningInstances returns the number of running processes matching a given profile
func CountRunningInstances(p Profile, running []RunningProcess) int {
	count := 0
	for _, proc := range running {
		for _, m := range p.Matchers {
			switch m.Kind {
			case MatchExecutablePath:
				if proc.Executable == m.Pattern || strings.HasSuffix(proc.Executable, "/"+m.Pattern) {
					count++
					break
				}
			case MatchProcessName:
				if strings.EqualFold(proc.Name, m.Pattern) {
					count++
					break
				}
			}
		}
	}
	return count
}
