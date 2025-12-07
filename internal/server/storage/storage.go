// Package storage provides storage-related routes.
package storage

import (
	"codesfer/pkg/api"
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
	storageHandler.HandleFunc("DELETE /remove", func(w http.ResponseWriter, r *http.Request) {
		if username := r.Header.Get("X-Username"); username != "" {
			log.Printf("[/storage/remove] user %s is trying to remove objects, including key %s", username, r.URL.Query()["key"])
			remove(w, r, username, r.URL.Query()["key"])
			return
		}
		http.Error(w, "unauthorized, only authorized users can remove", http.StatusUnauthorized)
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
	response := api.ListResponse{}
	for _, obj := range objs {
		response = append(response, api.SingleObject{
			Key:       obj.ID,
			Password:  obj.Password,
			Path:      obj.Path,
			CreatedAt: obj.CreatedAt,
		})
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
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

	// Make sure unique filename per user
	files, err := getFiles(username)
	if err != nil {
		http.Error(w, "failed to get existing files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Auto rename file if conflict by adding _1, _2, ...
	idx := 1
	haveFile, err := haveFile(username, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if haveFile {
		for {
			conflict := false
			for _, f := range files {
				if f.Filename == fmt.Sprintf("%s_%d", path, idx) {
					conflict = true
					log.Printf("[/storage/upload] path conflict, trying new filename: %s", fmt.Sprintf("%s_%d", path, idx))
				}
			}
			if !conflict {
				path = fmt.Sprintf("%s_%d", path, idx)
				log.Printf("[/storage/upload] rename complete, new filename: %s", path)
				break
			}
			idx++
		}
	}
	// Rename complete

	uid, err := opupload(r.Context(), file, header.Size, key, username, password, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.UploadResponse{
		Uid:  uid,
		Path: path,
	})
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

func remove(w http.ResponseWriter, r *http.Request, username string, keys []string) {
	resp := api.RemoveResponse{Results: make(map[string]string)}
	for _, key := range keys {
		// First, remove from indexdb
		path, err := removeByID(username, key)
		if err != nil {
			resp.Results[key] = "error removing from indexdb: " + err.Error()
			continue
		}
		log.Printf("[/storage/remove] object removed from indexdb (key: %s, path: %s) ", key, path)
		// Then, remove from object storage
		err = opremove(r.Context(), path)
		if err != nil {
			resp.Results[key] = "error removing from object storage: " + err.Error()
			continue
		}
		log.Printf("[/storage/remove] object removed from object storage")
		resp.Results[key] = "removed"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
