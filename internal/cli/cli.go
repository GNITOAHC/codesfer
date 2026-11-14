// Package cli provides command-line interface utilities for the application.
package cli

import (
	"os/exec"
	"strings"

	"github.com/gnitoahc/codesfer/internal/client"
)

// shareLink returns the download link for key: the frontend when it's up,
// otherwise the API server.
func shareLink(key string) string {
	if client.ClientURLHealthy() {
		return client.ClientURL + "/d/" + key
	}
	return client.BaseURL + "/download/" + key + ".zip"
}

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
