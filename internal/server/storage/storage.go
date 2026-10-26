// Package storage provides storage-related routes.
package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gnitoahc/codesfer/internal/constants"
	"github.com/gnitoahc/codesfer/internal/server/auth"
	"github.com/gnitoahc/codesfer/pkg/api"
	"github.com/gnitoahc/codesfer/pkg/object"
)

var objectStorage object.ObjectStorage

// maxUploadMemory caps how much of a multipart request body is held in RAM;
// anything larger spills to a temp file. It is not an upload size limit.
const maxUploadMemory = 32 << 20

func StorageHandler(driver, source string, objStorage object.ObjectStorage) http.Handler {
	// Setup indexdb
	err := connect(driver, source)
	if err != nil {
		panic(err)
	}

	// Setup object storage
	objectStorage = objStorage

	storageHandler := http.NewServeMux()
	storageHandler.HandleFunc("POST /upload", upload) // anonymous allowed
	storageHandler.HandleFunc("POST /upload/chunk", requireUser("upload", uploadChunk))
	storageHandler.HandleFunc("GET /download", download)
	storageHandler.HandleFunc("GET /list", requireUser("list", func(w http.ResponseWriter, r *http.Request, _ string) { list(w, r) }))
	storageHandler.HandleFunc("DELETE /remove", requireUser("remove", func(w http.ResponseWriter, r *http.Request, username string) {
		remove(w, r, username, r.URL.Query().Get("key"))
	}))
	storageHandler.HandleFunc("GET /info", info)
	storageHandler.HandleFunc("PATCH /settings", requireUser("change settings", updateSettings))
	return storageHandler
}

// requireUser rejects unauthenticated requests and passes the caller's username
// (set by the auth middleware) to the handler. action names the operation in
// the error message.
func requireUser(action string, handler func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.Header.Get("X-Username")
		if username == "" {
			http.Error(w, "unauthorized, only authorized users can "+action, http.StatusUnauthorized)
			return
		}
		handler(w, r, username)
	}
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
			Key:         obj.ID,
			Password:    obj.Password,
			Path:        obj.IdxPath,
			CreatedAt:   obj.CreatedAt,
			AccessScope: obj.AccessScope,
		})
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// upload compressed file to object storage and return uid; path: username/<dir>/filename
// file: multipart/form-data
// key: optional
// path: optional
// password: optional
// force: optional
//
// Without a session the upload is anonymous: rate limited per IP, capped at
// UploadChunkSize, always public with a generated key, and expiring after
// AnonUploadTTL. key, force and access are ignored.
func upload(w http.ResponseWriter, r *http.Request) {
	username := r.Header.Get("X-Username")
	anonymous := username == ""
	if anonymous {
		if !allowAnonUpload(auth.ClientIP(r), time.Now()) {
			http.Error(w, "too many anonymous uploads, try again later or log in", http.StatusTooManyRequests)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, constants.UploadChunkSize+anonFormSlack)
	}

	if err := r.ParseMultipartForm(maxUploadMemory); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, fmt.Sprintf("anonymous uploads are limited to %d MB, log in for larger files", constants.UploadChunkSize>>20), http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	rec := uploadRecord{
		Username: username,
		Key:      r.FormValue("key"),
		IdxPath:  r.FormValue("path"),
		Password: r.FormValue("password"),
		Meta:     r.FormValue("meta"),
	}
	if anonymous {
		// Everyone shares anonUsername, so nothing here may let one anonymous
		// upload claim or replace another's: no custom key, no overwrite.
		name := sanitizeFilename(header.Filename)
		if name == "" {
			name = "file"
		}
		id, err := generateID(objPathIDLen)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rec.Username = anonUsername
		rec.Key = ""
		rec.KeyLen = anonKeyLen
		rec.IdxPath = id + "/" + name // unique by construction, skips uniqueIdxPath
		rec.Scope = ScopePublic
		rec.ExpiresAt = time.Now().Add(constants.AnonUploadTTL).Unix()
	} else {
		if rec.Scope, err = normalizeScope(r.FormValue("access")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if rec.IdxPath == "" || rec.IdxPath == "." || rec.IdxPath == "/" { // path gaurd
			rec.IdxPath = header.Filename
		}
		// Make sure the filename is unique per user
		if rec.IdxPath, err = uniqueIdxPath(username, rec.IdxPath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rec.Overwrite = r.FormValue("force") == "true"
	}
	log.Printf("[/storage/upload] user %s is trying to upload file with key: %s; path: %s", rec.Username, rec.Key, rec.IdxPath)

	uid, err := opupload(r.Context(), file, rec)
	if err != nil {
		http.Error(w, err.Error(), recordStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.UploadResponse{
		Uid:       uid,
		Path:      rec.IdxPath,
		ExpiresAt: rec.ExpiresAt,
	})
}

// download will return the archived file to user according to the key
// key: <uid> || <username>/<uid> || <username>/<path>
func download(w http.ResponseWriter, r *http.Request) {
	// Anonymous downloads allowed; checkAccess gates by scope and password.
	currentUser := r.Header.Get("X-Username")

	key := r.URL.Query().Get("key")
	pwd := r.URL.Query().Get("password")
	log.Printf("[/storage/download] user %s is trying to download object, key: %s", currentUser, key)

	obj, err := findObject(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if obj == nil {
		http.Error(w, "object not found", http.StatusNotFound)
		return
	}
	log.Printf("  Object found: uid: %s, owner: %s, obj path: %s", obj.ID, obj.Username, obj.ObjPath)

	if status, msg, gate := checkAccess(obj, currentUser, pwd); status != 0 {
		log.Printf("  Access denied (%s), returning %d", gate, status)
		writeJSONError(w, status, msg, gate)
		return
	}

	log.Printf("  resp: username: %s, filename: %s, path: %s, uid: %s", obj.Username, obj.IdxPath, obj.ObjPath, obj.ID)

	// ============================
	// Download from Object Storage
	// ============================

	meta, body, err := objectStorage.Get(r.Context(), obj.ObjPath, nil)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, object.ErrNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", sanitizeFilename(obj.ObjPath)))
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

// remove deletes a single object. Callers wanting to remove several objects
// send one request per key (the CLI does so in parallel).
func remove(w http.ResponseWriter, r *http.Request, username, key string) {
	log.Printf("[/storage/remove] user %s is trying to remove object, key: %s", username, key)
	if key == "" {
		writeJSONError(w, http.StatusBadRequest, "missing key", "")
		return
	}

	// First, remove from indexdb
	path, err := removeByID(username, key)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("  username: %s, key: %s; not found in indexdb", username, key)
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("key %s not found for user %s", key, username), "")
		return
	} else if err != nil {
		log.Printf("  username: %s, key: %s; error removing from indexdb: %v", username, key, err)
		writeJSONError(w, http.StatusInternalServerError, "error removing key: "+err.Error(), "")
		return
	}
	log.Printf("  username: %s, key: %s, path: %s; removed from indexdb", username, key, path)

	// Then, remove from object storage
	if err := opremove(r.Context(), path); err != nil {
		log.Printf("  key: %s, path: %s; error removing from object storage: %v", key, path, err)
		writeJSONError(w, http.StatusInternalServerError, "error removing from object storage: "+err.Error(), "")
		return
	}
	log.Printf("  key: %s, path: %s; removed from object storage", key, path)

	w.WriteHeader(http.StatusNoContent)
}

// updateSettings changes the mutable settings of an object (id, filename,
// metadata description, access scope). Owner only; nil request fields are
// left unchanged.
func updateSettings(w http.ResponseWriter, r *http.Request, username string) {
	key := r.URL.Query().Get("key")
	log.Printf("[/storage/settings] user %s is updating object, key: %s", username, key)

	obj, err := findObject(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Report not-found for other users' objects too, so ownership isn't leaked.
	if obj == nil || obj.Username != username {
		http.Error(w, "object not found", http.StatusNotFound)
		return
	}

	var req api.UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	oldID := obj.ID
	if req.Key != nil && *req.Key != "" && *req.Key != obj.ID {
		taken, err := keyExists(*req.Key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if taken {
			http.Error(w, "key already in use: "+*req.Key, http.StatusConflict)
			return
		}
		obj.ID = *req.Key
	}
	if req.IdxPath != nil && *req.IdxPath != "" && *req.IdxPath != obj.IdxPath {
		have, err := haveFile(username, *req.IdxPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if have {
			http.Error(w, "filename already in use: "+*req.IdxPath, http.StatusConflict)
			return
		}
		obj.IdxPath = *req.IdxPath
	}
	if req.AccessScope != nil {
		scope, err := normalizeScope(*req.AccessScope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		obj.AccessScope = scope
	}
	if req.Desc != nil {
		metadata := map[string]any{}
		if obj.Metadata != "" {
			if err := json.Unmarshal([]byte(obj.Metadata), &metadata); err != nil {
				log.Printf("  Warning: failed to parse metadata JSON: %v", err)
			}
		}
		if *req.Desc == "" {
			delete(metadata, "desc")
		} else {
			metadata["desc"] = *req.Desc
		}
		b, err := json.Marshal(metadata)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		obj.Metadata = string(b)
	}

	if err := updateObject(oldID, obj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("  updated: id: %s -> %s, filename: %s, scope: %s", oldID, obj.ID, obj.IdxPath, obj.AccessScope)

	var metadata map[string]any
	if obj.Metadata != "" {
		if err := json.Unmarshal([]byte(obj.Metadata), &metadata); err != nil {
			log.Printf("  Warning: failed to parse metadata JSON: %v", err)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.InspectResponse{
		Key:         obj.ID,
		Owner:       obj.Username,
		Path:        obj.IdxPath,
		CreatedAt:   obj.CreatedAt,
		Protected:   obj.Password != "",
		AccessScope: obj.AccessScope,
		Metadata:    metadata,
		ExpiresAt:   obj.ExpiresAt,
	})
}

// info returns metadata about a code snippet without downloading it
// key: <uid> || <username>/<uid> || <username>/<path>
func info(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	pwd := r.URL.Query().Get("password")
	currentUser := r.Header.Get("X-Username")
	log.Printf("[/storage/info] user %s is inspecting object, key: %s", currentUser, key)

	obj, err := findObject(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if obj == nil {
		http.Error(w, "object not found", http.StatusNotFound)
		return
	}
	log.Printf("  Object found: uid: %s, owner: %s, obj path: %s", obj.ID, obj.Username, obj.ObjPath)

	if status, msg, gate := checkAccess(obj, currentUser, pwd); status != 0 {
		log.Printf("  Access denied (%s), returning %d", gate, status)
		writeJSONError(w, status, msg, gate)
		return
	}

	// Parse metadata JSON
	var metadata map[string]any
	if obj.Metadata != "" {
		if err := json.Unmarshal([]byte(obj.Metadata), &metadata); err != nil {
			log.Printf("  Warning: failed to parse metadata JSON: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.InspectResponse{
		Key:         obj.ID,
		Owner:       obj.Username,
		Path:        obj.IdxPath,
		CreatedAt:   obj.CreatedAt,
		Protected:   obj.Password != "",
		AccessScope: obj.AccessScope,
		Metadata:    metadata,
		ExpiresAt:   obj.ExpiresAt,
	})
}
