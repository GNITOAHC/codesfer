package client

import (
	"codesfer/pkg/api"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	configDir   = ".codesfer" // This should be in the user's home directory
	sessionFile = "session"   // This should be in the config directory
)

// makePaths will make sure the given directory exists, if not, it will be created
func makePaths(path string) error {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return err
	}
	return nil
}

// ReadSessionID returns the SessionID if the user is logged in,
// or an empty string if not.
func ReadSessionID() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	sessionFile := filepath.Join(home, configDir, sessionFile)
	if err := makePaths(filepath.Dir(sessionFile)); err != nil {
		log.Fatal(err)
	}

	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return ""
	}

	sessionID := strings.TrimSpace(string(data))
	return sessionID
}

func WriteSessionID(sessionID string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	sessionFile := filepath.Join(home, configDir, sessionFile)
	if err := makePaths(filepath.Dir(sessionFile)); err != nil {
		log.Fatal(err)
	}

	return os.WriteFile(sessionFile, []byte(sessionID), 0600)
}

func RemoveSessionID() error {
	return WriteSessionID("")
}

func Login(email, password string) (string, error) {
	url := BaseURL + "/auth/login"
	body := `{"email": "` + email + `", "password": "` + password + `"}`
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read plain text from response body
		errmsg, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		return "", errors.New(string(errmsg))
	}

	sessionID := strings.TrimSpace(resp.Header.Get("X-Session-ID"))
	return sessionID, nil
}

func Logout(sessionID string) error {
	url := BaseURL + "/auth/logout"
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+sessionID)

	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// log.Print("Logout failed, status code: ", resp.StatusCode)
		// Read plain text from response body
		errmsg, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		return errors.New(string(errmsg))
	}

	return nil
}

func Register(email, password, username string) error {
	url := BaseURL + "/auth/register"
	body := fmt.Sprintf(`{"email": "%s", "password": "%s", "username": "%s"}`, email, password, username)
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		log.Print("Register failed, status code: ", resp.StatusCode)
		// Read plain text from response body
		errmsg, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		return errors.New(string(errmsg))
	}

	return nil
}

func UsernameAvailable(username string) (bool, error) {
	url := BaseURL + "/auth/username?username=" + username
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return false, errors.New("Username is already taken")
	}
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}

	return false, nil
}

func AccountInfo(sessionID string) (*api.AccountResponse, error) {
	url := BaseURL + "/auth/me?session_id=" + sessionID
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read plain text from response body
		errmsg, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		return nil, errors.New(string(errmsg))
	}

	var account api.AccountResponse
	err = json.NewDecoder(resp.Body).Decode(&account)
	if err != nil {
		return nil, err
	}

	return &account, nil
}
