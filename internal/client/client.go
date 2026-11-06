// Package client contains CLI-only helpers: HTTP calls to the server, session storage, compression.
package client

import "time"

const (
	baseURLFile   = "base_url"
	clientURLFile = "client_url"
)

var (
	ConfigDir   = ".codesfer" // This should be in the user's home directory
	SessionFile = "session"   // This should be in the config directory

	BaseURL = "https://api.codesfer.io" // overwrite with -ldflags -X github.com/gnitoahc/codesfer/internal/client.BaseURL=<default URL>
	// frontend that proxies downloads at /d/<key>
	// overwrite with -ldflags -X github.com/gnitoahc/codesfer/internal/client.ClientURL=<default URL>
	// If ClientURL is empty string, it'll fallback to BaseURL.
	ClientURL = "https://codesfer.io"
)

// ClientURLHealthy reports whether the frontend at ClientURL is reachable.
func ClientURLHealthy() bool {
	if ClientURL == "" {
		return false
	}
	c := GetHTTPClient()
	c.Timeout = 1 * time.Second
	resp, err := c.Head(ClientURL + "/healthy")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}
