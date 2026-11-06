package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gnitoahc/codesfer/pkg/api"
)

// doURL sends req, with the session when there is one, and decodes a
// successful JSON response into out (nil for no body).
func doURL(req *http.Request, sessionID string, want int, out any) error {
	if sessionID != "" {
		req.Header.Set("Authorization", "Bearer "+sessionID)
	}
	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		return fmt.Errorf("%s: %s", resp.Status, serverError(resp.Body))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// URLNew creates a short url for target. An empty sessionID creates an
// anonymous, expiring link.
func URLNew(sessionID, target, custom, password string) (*api.ShortURLResponse, error) {
	form := url.Values{"target": {target}, "custom": {custom}, "password": {password}}
	req, err := http.NewRequest("POST", BaseURL+"/url/new", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out api.ShortURLResponse
	return &out, doURL(req, sessionID, http.StatusOK, &out)
}

func URLList(sessionID string) (api.ShortURLListResponse, error) {
	req, err := http.NewRequest("GET", BaseURL+"/url/list", nil)
	if err != nil {
		return nil, err
	}
	var out api.ShortURLListResponse
	return out, doURL(req, sessionID, http.StatusOK, &out)
}

// URLRemove removes a short url by key (<handle> || <username>/<custom>).
// Safe for concurrent use.
func URLRemove(sessionID, key string) error {
	req, err := http.NewRequest("DELETE", BaseURL+"/url/remove?key="+url.QueryEscape(key), nil)
	if err != nil {
		return err
	}
	return doURL(req, sessionID, http.StatusNoContent, nil)
}

// URLGet returns a short url's metadata by key (<handle> || <username>/<custom>).
func URLGet(sessionID, key, password string) (*api.ShortURLResponse, error) {
	q := url.Values{"key": {key}}
	if password != "" {
		q.Set("password", password)
	}
	req, err := http.NewRequest("GET", BaseURL+"/url/info?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var out api.ShortURLResponse
	return &out, doURL(req, sessionID, http.StatusOK, &out)
}
