// Package client contains CLI-only helpers: HTTP calls to the server, session storage, compression.
package client

const (
	configDir   = ".codesfer" // This should be in the user's home directory
	sessionFile = "session"   // This should be in the config directory
	baseURLFile = "base_url"
)

var (
	BaseURL = "https://api.codesfer.io" // overwrite with -ldflags -X codesfer/internal/client.BaseURL=<default URL>
)
