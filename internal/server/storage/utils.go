package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
)

func generateID(n int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_+-="
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range n {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}

// objPath returns the path to object inside object storage
func objPath(username, path string) string {
	return fmt.Sprintf("%s/%s", username, strings.Trim(path, "/"))
}

// opupload will upload a file to object storage cloud and insert a record to database
func opupload(ctx context.Context, file io.Reader, size int64, key, username, password, path string) (string, error) {
	const multipartThreshold = 100 << 20 // 100 MB

	if key == "" {
		uid, err := generateID(4)
		if err != nil {
			return "", errors.New("[op upload] [generate uid] generate uid failed: " + err.Error())
		}
		key = uid
	}

	objectPath := objPath(username, path)

	err := insert(key, username, path, password, objectPath)
	if err != nil {
		return "", errors.New("[op upload] [insert] insert failed: " + err.Error())
	}

	// Only upload after insert is successfull
	if size > multipartThreshold {
		log.Print("Stream via multipart")
		if _, err := objectStorage.MultipartPut(ctx, objectPath, file, 8<<20, nil); err != nil {
			return "", errors.New("[op upload] [multipart] multipart upload failed: " + err.Error())
		}
	} else {
		log.Print("Single PutObject")
		if _, err := objectStorage.Put(ctx, objectPath, file, -1, "", nil); err != nil {
			return "", errors.New("[op upload] [single putobject] upload failed: " + err.Error())
		}
	}

	return key, nil
}

// sanitizeFilename extracts the base filename (safe for headers).
func sanitizeFilename(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "file"
	}
	return parts[len(parts)-1]
}
