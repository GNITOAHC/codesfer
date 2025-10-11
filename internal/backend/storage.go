package backend

import (
	"bytes"
	"crypto/rand"
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

func Push(form PushForm, zipFile string, anon bool) (string, error) {
	// Open the file
	file, err := os.Open(zipFile)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Prepare multipart writer
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add the file field
	part, err := writer.CreateFormFile("file", filepath.Base(zipFile))
	if err != nil {
		return "", err
	}
	if _, err = io.Copy(part, file); err != nil {
		return "", err
	}

	// Add the customName field (key)
	if form.Key != "" {
		if err = writer.WriteField("key", form.Key); err != nil {
			return "", err
		}
	}

	// Add the customName field (path)
	if form.Path != "" {
		if err = writer.WriteField("path", form.Path); err != nil {
			return "", err
		}
	}

	// Add the customName field (password)
	if form.Password != "" {
		if err = writer.WriteField("password", form.Password); err != nil {
			return "", err
		}
	}

	// Close writer to finalize the body
	if err = writer.Close(); err != nil {
		return "", err
	}

	// Create request
	var route string
	if anon {
		route = "/anonymous/upload"
	} else {
		route = "/storage/upload"
	}
	req, err := http.NewRequest("POST", BaseURL+route, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+ReadSessionID())
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned status: %s", resp.Status)
	}

	// Parse JSON response
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Extract "uid"
	uid, ok := result["uid"].(string)
	if !ok {
		return "", fmt.Errorf("uid field missing or not a string")
	}

	return uid, nil
}

type Object struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Filename  string `json:"filename"`
	Password  string `json:"password"`
	Path      string `json:"path"`
	CreatedAt string `json:"created_at"`
}

func List(sessionID string) ([]Object, error) {
	url := BaseURL + "/storage/list"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sessionID)

	resp, err := http.DefaultClient.Do(req)
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

	var objects []Object
	if err := json.NewDecoder(resp.Body).Decode(&objects); err != nil {
		return nil, err
	}

	return objects, nil
}

func generateID(n int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range n {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}

// Pull a file and automatically extract
// key: <uid> || <username>/<uid> || <username>/<path>
func Pull(sessionID, key, password string) (string, error) {
	url := BaseURL + "/storage/download" + "?key=" + key + "&password=" + password
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+sessionID)

	resp, err := http.DefaultClient.Do(req)
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

	randID, err := generateID(5)
	if err != nil {
		return "", err
	}
	file, err := os.Create("./codesfer_download_" + randID + ".zip")
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", err
	}

	fmt.Printf("File downloaded successfully: %s\n", file.Name())
	return file.Name(), nil
}
