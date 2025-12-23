package client

import (
	"bytes"
	"codesfer/pkg/api"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

type PushForm struct {
	Key      string
	Path     string
	Password string
}

func Push(form PushForm, zipFile string) (*api.UploadResponse, error) {
	// Open the file
	file, err := os.Open(zipFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Prepare multipart writer
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add the file field
	part, err := writer.CreateFormFile("file", filepath.Base(zipFile))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(part, file); err != nil {
		return nil, err
	}

	// Add the customName field (key)
	if form.Key != "" {
		if err = writer.WriteField("key", form.Key); err != nil {
			return nil, err
		}
	}

	// Add the customName field (path)
	if form.Path != "" {
		if err = writer.WriteField("path", form.Path); err != nil {
			return nil, err
		}
	}

	// Add the customName field (password)
	if form.Password != "" {
		if err = writer.WriteField("password", form.Password); err != nil {
			return nil, err
		}
	}

	// Close writer to finalize the body
	if err = writer.Close(); err != nil {
		return nil, err
	}

	// Create request
	route := "/storage/upload"
	req, err := http.NewRequest("POST", BaseURL+route, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+ReadSessionID())
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
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
		return nil, fmt.Errorf("server returned status: %s; error: %s", resp.Status, errmsg)
	}

	// Parse JSON response
	var result api.UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func List(sessionID string) (api.ListResponse, error) {
	url := BaseURL + "/storage/list"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sessionID)

	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errmsg, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		return nil, errors.New(string(errmsg))
	}

	var objects api.ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&objects); err != nil {
		return nil, err
	}

	return objects, nil
}

// Pull a file and automatically extract
// key: <uid> || <username>/<uid> || <username>/<path>
func Pull(sessionID, key, password string) (string, error) {
	prefix := "/storage/download"
	url := BaseURL + prefix + "?key=" + key + "&password=" + password
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+sessionID)

	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errmsg, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Download failed: %s\n", err.Error())
			panic(err)
		}
		return "", errors.New(string(errmsg))
	}

	file, err := os.CreateTemp("", "codesfer_download_*.zip")
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", err
	}

	return file.Name(), nil
}

// Remove files by their keys
func Remove(sessionID string, keys []string) (*api.RemoveResponse, error) {
	queryParam := ""
	for _, key := range keys {
		queryParam += "key=" + key + "&"
	}

	url := BaseURL + "/storage/remove?" + queryParam
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sessionID)

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

	var result api.RemoveResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
