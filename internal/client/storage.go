package client

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gnitoahc/codesfer/pkg/api"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

type PushForm struct {
	Key      string
	Path     string
	Password string
	Force    bool
	Access   string // access scope: owner | authenticated | public (empty = server default)
	Metadata map[string]any
}

// writeFormFields adds the optional PushForm fields shared by the plain and
// chunked upload requests to the multipart writer.
func writeFormFields(writer *multipart.Writer, form PushForm) error {
	fields := []struct{ name, value string }{
		{"key", form.Key},
		{"path", form.Path},
		{"password", form.Password},
		{"access", form.Access},
	}
	if form.Force {
		fields = append(fields, struct{ name, value string }{"force", "true"})
	}
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		if err := writer.WriteField(f.name, f.value); err != nil {
			return err
		}
	}
	if form.Metadata != nil {
		metaJSON, err := json.Marshal(form.Metadata)
		if err != nil {
			return err
		}
		return writer.WriteField("meta", string(metaJSON))
	}
	return nil
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

	if err = writeFormFields(writer, form); err != nil {
		return nil, err
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

// chunkSize is the maximum bytes per chunk. Kept under Cloudflare's 100 MB
// request-body limit with a comfortable margin.
const chunkSize = 90 << 20 // 90 MB

// generateUploadID returns a random hex string used to correlate chunks on the server.
func generateUploadID() string {
	b := make([]byte, 8)
	rand.Read(b) //nolint:errcheck — crypto/rand.Read never fails on supported platforms
	return fmt.Sprintf("%x", b)
}

// PushChunked splits zipFile into <=90 MB chunks and uploads them sequentially.
// The server reassembles the chunks before writing to object storage.
// Use this when the compressed archive exceeds Cloudflare's 100 MB body limit.
func PushChunked(form PushForm, zipFile string) (*api.UploadResponse, error) {
	file, err := os.Open(zipFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	totalSize := info.Size()
	totalChunks := int((totalSize + chunkSize - 1) / chunkSize)
	uploadID := generateUploadID()

	log.Printf("Chunked upload: %d chunk(s) of up to 90 MB each (upload id: %s)", totalChunks, uploadID)

	for i := range totalChunks {
		log.Printf("Uploading chunk %d/%d ...", i+1, totalChunks)

		offset := int64(i) * chunkSize
		size := int64(chunkSize)
		if offset+size > totalSize {
			size = totalSize - offset
		}
		chunk := io.NewSectionReader(file, offset, size)

		resp, err := pushChunk(form, uploadID, i, totalChunks, chunk)
		if err != nil {
			return nil, err
		}
		if resp != nil {
			return resp, nil
		}
	}

	return nil, errors.New("chunked upload finished but server never returned a final response")
}

// pushChunk sends a single chunk to POST /storage/upload/chunk.
// It returns nil when the server acknowledges a partial chunk (202 Accepted),
// or the completed UploadResponse when the last chunk triggers assembly (200 OK).
func pushChunk(form PushForm, uploadID string, chunkIndex, totalChunks int, chunk io.Reader) (*api.UploadResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("upload_id", uploadID); err != nil {
		return nil, err
	}
	if err := writer.WriteField("chunk_index", strconv.Itoa(chunkIndex)); err != nil {
		return nil, err
	}
	if err := writer.WriteField("total_chunks", strconv.Itoa(totalChunks)); err != nil {
		return nil, err
	}

	part, err := writer.CreateFormFile("file", fmt.Sprintf("chunk_%d", chunkIndex))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(part, chunk); err != nil {
		return nil, err
	}

	if err := writeFormFields(writer, form); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", BaseURL+"/storage/upload/chunk", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+ReadSessionID())
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusAccepted:
		return nil, nil // chunk stored, waiting for the rest
	case http.StatusOK:
		var result api.UploadResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}
		return &result, nil
	default:
		errmsg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status: %s; error: %s", resp.Status, errmsg)
	}
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

// UpdateSettings changes an object's settings (new key, filename, description,
// access scope) via PATCH /storage/settings. Nil fields are left unchanged.
func UpdateSettings(sessionID, key string, settings api.UpdateSettingsRequest) (*api.InspectResponse, error) {
	body, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PATCH", BaseURL+"/storage/settings?key="+key, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sessionID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errmsg, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, errors.New(string(errmsg))
	}

	var result api.InspectResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Inspect retrieves metadata about a code snippet without downloading it
func Inspect(sessionID, key, password string) (*api.InspectResponse, error) {
	url := BaseURL + "/storage/info?key=" + key
	if password != "" {
		url += "&password=" + password
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if sessionID != "" {
		req.Header.Set("Authorization", "Bearer "+sessionID)
	}

	resp, err := GetHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errmsg, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, errors.New(string(errmsg))
	}

	var result api.InspectResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
