package backend

import (
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var (
	BaseURL string // -ldflags -X codesfer/internal/backend.BaseURL=<default URL>
)

const (
	baseURLFile = "base_url"
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	baseURLFile := filepath.Join(home, configDir, baseURLFile)

	if err := makePaths(filepath.Dir(baseURLFile)); err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(baseURLFile); os.IsNotExist(err) {
		// log.Printf("Fallback to %s", BaseURL)
		return
	}

	byteBaseURL, err := os.ReadFile(baseURLFile)
	if err != nil {
		log.Fatal(err)
	}
	stringBaseURL := strings.TrimSuffix(strings.TrimSpace(string(byteBaseURL)), "/")
	_, err = url.ParseRequestURI(string(byteBaseURL))
	if err != nil {
		log.Fatal(err)
	}
	BaseURL = stringBaseURL
}
