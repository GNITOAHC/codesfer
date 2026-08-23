package storage

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gnitoahc/codesfer/internal/constants"
)

// checkAccess decides whether username (empty = anonymous) may read obj with
// the supplied password. Returns status 0 when allowed, otherwise the HTTP
// status plus msg/gate for writeJSONError; the gate ("auth" or "password")
// tells the frontend which prompt to show. Owner-scoped objects are reported
// as 404 to other users, and the owner bypasses the password gate.
func checkAccess(obj *Object, username, password string) (status int, msg, gate string) {
	if username != "" && username == obj.Username {
		return 0, "", ""
	}
	switch obj.AccessScope {
	case ScopeOwner:
		if username == "" {
			return http.StatusUnauthorized, "login required", "auth"
		}
		return http.StatusNotFound, "not found", ""
	case ScopeAuthenticated:
		if username == "" {
			return http.StatusUnauthorized, "login required", "auth"
		}
	}
	if obj.Password != "" && password != obj.Password {
		return http.StatusUnauthorized, "password required", "password"
	}
	return 0, "", ""
}

func writeJSONError(w http.ResponseWriter, status int, msg, gate string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]string{"error": msg}
	if gate != "" {
		resp["gate"] = gate
	}
	json.NewEncoder(w).Encode(resp)
}

// generateID generates a random string of length n
func generateID(n int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range n {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}

// findObject resolves a client-facing key (<uid> || <username>/<uid> || <username>/<path>)
// to an object, or nil when nothing matches.
func findObject(key string) (*Object, error) {
	uid, username, path := parseKey(key)
	if obj, err := get(uid); obj != nil || err != nil {
		return obj, err
	}
	return getByUsernamePath(username, path)
}

// parseKey extracts the key from a path
// key: <uid> || <username>/<uid> || <username>/<path>
func parseKey(key string) (string, string, string) {
	// If contains multiple slashes, it must be username/path/path
	// If contains one slash, it could be either username/uid or username/path
	// If contains no slash, it must be uid
	if !strings.Contains(key, "/") {
		return key, "", "" // uid
	}
	parts := strings.SplitN(key, "/", 2)
	username := parts[0]
	if strings.Contains(parts[1], "/") {
		return "", username, parts[1] // username/path
	} else {
		return parts[1], username, parts[1] // username/path or username/uid
	}
}

// objPath returns the path to object inside object storage
func objPath(username, path string) string {
	return fmt.Sprintf("%s/%s", username, strings.Trim(path, "/"))
}

// opupload will upload a file to object storage cloud and insert a record to database
func opupload(ctx context.Context, file io.Reader, size int64, key, username, password, path string, overwrite bool, metadata, scope string) (string, error) {
	var err error

	if key == "" {
		uid, err := generateID(4)
		if err != nil {
			return "", errors.New("[op upload] [generate uid] generate uid failed: " + err.Error())
		}
		key = uid
	}

	objectPath := objPath(username, path)

	if overwrite {
		log.Print("[op upload] overwrite is true, upsert record")
		err = upsert(key, username, path, password, objectPath, metadata, scope)
	} else {
		log.Print("[op upload] overwrite is false, insert record")
		err = insert(key, username, path, password, objectPath, metadata, scope)
	}
	if err != nil {
		return "", errors.New("[op upload] [insert] insert failed: " + err.Error())
	}

	// Only upload after insert is successfull.
	// Note: the multipart branch is unreachable in practice — the client caps
	// non-chunked uploads at 90 MB, which is below multipartThreshold (100 MB).
	// Large files go through the chunked upload path (StreamingWriter) instead.
	if constants.UploadChunkSize < size {
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

func opremove(ctx context.Context, path string) error {
	err := objectStorage.Delete(ctx, path)
	if err != nil {
		return errors.New("[op remove] [delete] delete failed: " + err.Error())
	}
	return nil
}

// sanitizeFilename extracts the base filename (safe for headers).
func sanitizeFilename(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "file"
	}
	return parts[len(parts)-1]
}
