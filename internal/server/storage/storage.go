// Package storage provides storage-related routes.
package storage

import (
	"codesfer/pkg/object"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

var objectStorage object.ObjectStorage

func StorageHandler(driver, source string, objStorage object.ObjectStorage) http.Handler {
	// Setup indexdb
	err := connect(driver, source)
	if err != nil {
		panic(err)
	}

	// Setup object storage
	objectStorage = objStorage

	storageHandler := http.NewServeMux()
	storageHandler.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
		if username := r.Header.Get("X-Username"); username != "" {
			upload(w, r, username)
			return
		}
		http.Error(w, "unauthorized, only authorized users can upload", http.StatusUnauthorized)
	})
	storageHandler.HandleFunc("GET /download", download)
	storageHandler.HandleFunc("GET /list", func(w http.ResponseWriter, r *http.Request) {
		if username := r.Header.Get("X-Username"); username != "" {
			list(w, r)
			return
		}
		http.Error(w, "unauthorized, only authorized users can list", http.StatusUnauthorized)
	})
	return storageHandler
}

func list(w http.ResponseWriter, r *http.Request) {
	username := r.Header.Get("X-Username")
	if username == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	log.Printf("[/storage/list] user %s is trying to list objects", username)
	objs, err := show(username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(objs)
}

// upload compressed file to R2 and return uid; path: username/<dir>/filename
// file: multipart/form-data
// key: optional
// path: optional
// password: optional
func upload(w http.ResponseWriter, r *http.Request, username string) {
	// Max upload size: 500 MB
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	key := r.FormValue("key")
	path := r.FormValue("path")
	password := r.FormValue("password")
	log.Printf("[/storage/upload] user %s is trying to upload file with key %s; path: %s; password: %s", username, key, path, password)

	filename := header.Filename
	if path != "" {
		filename = path
		log.Printf("[/storage/upload] user %s is trying to upload file with path %s", username, filename)
	}

	uid, err := opupload(r.Context(), file, header.Size, key, username, password, filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"uid": uid})
}

// download will return the archived file to user according to the key
// key: <uid> || <username>/<uid> || <username>/<path>
func download(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	pwd := r.URL.Query().Get("password")
	// If contains multiple slashes, it must be username/path/path
	// If contains one slash, it could be either username/uid or username/path
	// If contains no slash, it must be uid
	uid, username, path := func() (string, string, string) {
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
	}()

	log.Printf("[/storage/download] user %s is trying to download object %s", r.Header.Get("X-Username"), key)
	log.Printf("uid: %s, username: %s, path: %s", uid, username, path)

	var obj *Object
	var err error
	if obj, err = get(uid); obj != nil || err != nil {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("  Object found by uid: %s", obj.ID)
	} else {
		obj, err = getByUsernamePath(username, path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if obj != nil {
			log.Printf("  Object found by username/path: %s/%s; uid: %s", obj.Username, obj.Path, obj.ID)
		}
		if obj == nil {
			http.Error(w, "object not found", http.StatusNotFound)
			return
		}
	}

	if obj.Password != "" && pwd != obj.Password {
		log.Printf("Invalid password, returning StatusUnauthorized %d", http.StatusUnauthorized)
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}

	log.Printf("[/storage/download] user %s is downloading object", r.Header.Get("X-Username"))
	log.Printf("username: %s, filename: %s, path: %s, uid: %s", obj.Username, obj.Filename, obj.Path, obj.ID)

	// ============================
	// Download from Object Storage
	// ============================

	meta, body, err := objectStorage.Get(r.Context(), obj.Path, nil)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, object.ErrNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", sanitizeFilename(obj.Path)))
	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if meta.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	}

	if _, err := io.Copy(w, body); err != nil {
		log.Printf("download stream error: %v", err)
	}
}
