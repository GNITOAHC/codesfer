// Package cli provides command-line interface utilities for the application.
package cli

import (
	"os/exec"
	"strings"
)

// copyToClipboard tries the common clipboard tools; false if none worked.
func copyToClipboard(s string) bool {
	for _, c := range [][]string{{"pbcopy"}, {"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}, {"clip.exe"}} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(s)
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}
