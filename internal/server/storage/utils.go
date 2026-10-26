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

// objPath returns a fresh path inside object storage for an upload of path:
// <username>/<random>/<basename>. The random segment makes it unique per
// upload, so concurrent uploads never share bytes and a failed upload can
// delete its own object safely. The basename stays last because downloads
// name the file after it.
func objPath(username, path string) (string, error) {
	id, err := generateID(objPathIDLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/%s", username, id, sanitizeFilename(strings.Trim(path, "/"))), nil
}

// uniqueIdxPath returns path, or path_1 / path_2 / ... when the user already has
// a file under that name.
//
// one query per candidate, fine for the handful of collisions a real user hits.
// Add a single indexed LIKE query if someone uploads path_500.
func uniqueIdxPath(username, path string) (string, error) {
	for i := 0; ; i++ {
		candidate := path
		if i > 0 {
			candidate = fmt.Sprintf("%s_%d", path, i)
		}
		have, err := haveFile(username, candidate)
		if err != nil {
			return "", err
		}
		if !have {
			log.Printf("[unique path] %s -> %s", path, candidate)
			return candidate, nil
		}
	}
}

const (
	keyLen       = 4 // generated key length for signed-in uploads
	anonKeyLen   = 8 // longer for anonymous uploads, which are all public
	objPathIDLen = 8
	keyRetries   = 3 // attempts at a free generated key
)

// uploadRecord is the index record for an uploaded object.
type uploadRecord struct {
	Key       string // empty: generate one of KeyLen characters
	KeyLen    int    // 0 = keyLen
	Username  string
	IdxPath   string
	Password  string
	ObjPath   string
	Meta      string
	Scope     string
	Overwrite bool
	ExpiresAt int64 // unix seconds; 0 = never
}

// commitUpload records an object whose bytes are already in object storage
// and returns the key in use. Every upload path writes bytes first, record
// second: an orphaned object is invisible and RemoveOrphanedObject reclaims
// it, while a record without bytes is a broken link. If the record cannot be
// written, the object is deleted here (its path is unique to this upload).
func commitUpload(ctx context.Context, rec uploadRecord) (string, error) {
	key, err := saveRecord(rec)
	if err != nil {
		if delErr := objectStorage.Delete(context.WithoutCancel(ctx), rec.ObjPath); delErr != nil {
			log.Printf("[commit upload] cleanup of %s failed: %v", rec.ObjPath, delErr)
		}
		return "", err
	}
	return key, nil
}

// saveRecord writes rec, generating a key when none was given and retrying
// with a new one if it collides. A taken key or filename is reported as
// ErrKeyTaken / ErrKeyOwnedByAnotherUser / ErrPathTaken.
func saveRecord(rec uploadRecord) (string, error) {
	generate := rec.Key == ""
	if rec.KeyLen == 0 {
		rec.KeyLen = keyLen
	}
	for attempt := 1; ; attempt++ {
		key := rec.Key
		if generate {
			var err error
			if key, err = generateID(rec.KeyLen); err != nil {
				return "", fmt.Errorf("[save record] generate key failed: %w", err)
			}
		}

		var err error
		if rec.Overwrite {
			err = upsert(key, rec.Username, rec.IdxPath, rec.Password, rec.ObjPath, rec.Meta, rec.Scope, rec.ExpiresAt)
		} else {
			err = insert(key, rec.Username, rec.IdxPath, rec.Password, rec.ObjPath, rec.Meta, rec.Scope, rec.ExpiresAt)
		}
		if err == nil {
			return key, nil
		}
		if errors.Is(err, ErrKeyOwnedByAnotherUser) {
			return "", err
		}

		// Name the constraint that failed without parsing driver errors.
		if taken, _ := haveFile(rec.Username, rec.IdxPath); taken && !rec.Overwrite {
			return "", fmt.Errorf("%w: %s", ErrPathTaken, rec.IdxPath)
		}
		if taken, _ := keyExists(key); taken {
			if generate && attempt < keyRetries {
				continue
			}
			return "", fmt.Errorf("%w: %s", ErrKeyTaken, key)
		}
		return "", fmt.Errorf("[save record] insert failed: %w", err)
	}
}

// recordStatus maps a saveRecord error to its HTTP status.
func recordStatus(err error) int {
	if errors.Is(err, ErrKeyTaken) || errors.Is(err, ErrKeyOwnedByAnotherUser) || errors.Is(err, ErrPathTaken) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

// opupload stores file in object storage, then records it via commitUpload.
// rec.ObjPath is filled in here.
func opupload(ctx context.Context, file io.Reader, rec uploadRecord) (string, error) {
	var err error
	if rec.ObjPath, err = objPath(rec.Username, rec.IdxPath); err != nil {
		return "", err
	}
	if _, err := objectStorage.Put(ctx, rec.ObjPath, file, -1, "", nil); err != nil {
		return "", errors.New("[op upload] [put] upload failed: " + err.Error())
	}
	return commitUpload(ctx, rec)
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
