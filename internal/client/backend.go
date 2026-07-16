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
	// remove all \r or \n
	stringBaseURL := string(byteBaseURL)
	stringBaseURL = strings.ReplaceAll(stringBaseURL, "\r", "")
	stringBaseURL = strings.ReplaceAll(stringBaseURL, "\n", "")
	stringBaseURL = strings.TrimSuffix(strings.TrimSpace(stringBaseURL), "/")
	_, err = url.ParseRequestURI(stringBaseURL)
	if err != nil {
		log.Fatal(err)
	}
	BaseURL = stringBaseURL
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
