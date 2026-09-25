//go:build linux

package process

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type LinuxScanner struct {
	Root string
	UID  uint32
}

func NewScanner() Scanner {
	return &LinuxScanner{
		Root: "/proc",
		UID:  uint32(os.Getuid()),
	}
}

func (s *LinuxScanner) Scan(ctx context.Context) ([]RunningProcess, error) {
	root := s.Root
	if root == "" {
		root = "/proc"
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading proc root: %w", err)
	}

	myPID := os.Getpid()
	var results []RunningProcess

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 || pid == myPID {
			continue
		}

		procDir := filepath.Join(root, entry.Name())

		// Verify UID matches current user
		fileInfo, err := entry.Info()
		if err != nil {
			continue
		}
		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != s.UID {
			continue
		}

		// Read /proc/<pid>/comm (process name)
		commBytes, err := os.ReadFile(filepath.Join(procDir, "comm"))
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(string(commBytes))

		// Read /proc/<pid>/exe (executable link)
		exe, _ := os.Readlink(filepath.Join(procDir, "exe"))
		exe = strings.TrimSuffix(exe, " (deleted)")

		// Read /proc/<pid>/cmdline
		var cmd []string
		cmdBytes, err := os.ReadFile(filepath.Join(procDir, "cmdline"))
		if err == nil && len(cmdBytes) > 0 {
			parts := bytes.Split(cmdBytes, []byte{0})
			for _, p := range parts {
				if len(p) > 0 {
					cmd = append(cmd, string(p))
				}
			}
		}

		// Skip kernel threads or nameless items
		if exe == "" && comm == "" {
			continue
		}

		name := comm
		if name == "" && exe != "" {
			name = filepath.Base(exe)
		}

		results = append(results, RunningProcess{
			PID:        pid,
			Name:       name,
			Executable: exe,
			Command:    cmd,
		})
	}

	// Sort deterministically: Name -> Executable -> PID
	sort.Slice(results, func(i, j int) bool {
		if !strings.EqualFold(results[i].Name, results[j].Name) {
			return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
		}
		if results[i].Executable != results[j].Executable {
			return results[i].Executable < results[j].Executable
		}
		return results[i].PID < results[j].PID
	})

	return results, nil
}
