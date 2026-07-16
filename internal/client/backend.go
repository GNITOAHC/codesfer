package client

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gnitoahc/codesfer/pkg/version"
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	if err := makePaths(filepath.Join(home, configDir)); err != nil {
		log.Fatal(err)
	}

	if u := readURLOverride(filepath.Join(home, configDir, baseURLFile)); u != "" {
		BaseURL = u
	}
	if u := readURLOverride(filepath.Join(home, configDir, clientURLFile)); u != "" {
		ClientURL = u
	}
}

// readURLOverride reads a URL override file, returning "" if the file does not exist.
func readURLOverride(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ""
	}

	byteURL, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	// remove all \r or \n
	stringURL := string(byteURL)
	stringURL = strings.ReplaceAll(stringURL, "\r", "")
	stringURL = strings.ReplaceAll(stringURL, "\n", "")
	stringURL = strings.TrimSuffix(strings.TrimSpace(stringURL), "/")
	if _, err := url.ParseRequestURI(stringURL); err != nil {
		log.Fatal(err)
	}
	return stringURL
}

// uaTransport sets the CLI User-Agent on every outgoing request,
// replacing Go's default "Go-http-client/2.0".
type uaTransport struct {
	base http.RoundTripper
}

func (t *uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", "codesfer-cli/"+version.Version+" ("+runtime.GOOS+")")
	return t.base.RoundTrip(req)
}

// GetHTTPClient returns an HTTP client that respects proxy environment variables
// (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)
func GetHTTPClient() *http.Client {
	return &http.Client{
		Transport: &uaTransport{base: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}},
	}
}
