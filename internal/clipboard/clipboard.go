package clipboard

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ReadText attempts to read plain text from the system clipboard.
// It supports Wayland (wl-paste), X11 (xclip, xsel), macOS (pbpaste), and Windows.
// A strict timeout (500ms) ensures it never hangs or blocks the TUI render loop.
func ReadText() string {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		if path, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.CommandContext(ctx, path, "--no-newline")
		} else if path, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.CommandContext(ctx, path, "-selection", "clipboard", "-out")
		} else if path, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.CommandContext(ctx, path, "--clipboard", "--output")
		}
	case "darwin":
		if path, err := exec.LookPath("pbpaste"); err == nil {
			cmd = exec.CommandContext(ctx, path)
		}
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", "Get-Clipboard")
	}

	if cmd == nil {
		return ""
	}

	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\r\n")
}
