// Package storage provides storage-related routes.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gnitoahc/codesfer/pkg/api"
	"github.com/gnitoahc/codesfer/pkg/object"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
)

var objectStorage object.ObjectStorage

// chunkSession tracks an in-progress chunked upload.
type chunkSession struct {
	mu       sync.Mutex
	total    int
	received map[int]bool
	// Streaming mode: each chunk is uploaded to object storage as a multipart
	// part immediately upon arrival (enabled when objectStorage implements
	// object.StreamingWriter). Avoids assembling the full file before upload,
	// preventing Cloudflare 524 timeouts on large files.
	streaming    bool
	r2UploadID   string
	r2ObjectPath string
	r2Parts      map[int32]object.CompletedPart
	// Temp-file fallback: chunks accumulated on disk, assembled on last chunk.
	tempDir  string
	key      string
	path     string
	password string
	force    bool
	meta     string
	scope    string
	username string
}

var (
	chunkSessions   = make(map[string]*chunkSession)
	chunkSessionsMu sync.Mutex
)

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
	storageHandler.HandleFunc("POST /upload/chunk", func(w http.ResponseWriter, r *http.Request) {
		if username := r.Header.Get("X-Username"); username != "" {
			uploadChunk(w, r, username)
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
	storageHandler.HandleFunc("GET /info", info)
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
			Key:         obj.ID,
			Password:    obj.Password,
			Path:        obj.Path,
			CreatedAt:   obj.CreatedAt,
			AccessScope: obj.AccessScope,
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
// force: optional
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
	meta := r.FormValue("meta")
	scope, err := normalizeScope(r.FormValue("access"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if path == "" || path == "." || path == "/" { // path gaurd
		path = header.Filename
	}
	log.Printf("[/storage/upload] user %s is trying to upload file with key: %s; path: %s; password: %s", username, key, path, password)

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

	overwrite := false
	if r.FormValue("force") == "true" {
		overwrite = true
	}

	uid, err := opupload(r.Context(), file, header.Size, key, username, password, path, overwrite, meta, scope)
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
	// Anonymous downloads allowed; checkAccess gates by scope and password.
	currentUser := r.Header.Get("X-Username")

	key := r.URL.Query().Get("key")
	pwd := r.URL.Query().Get("password")
	uid, username, path := parseKey(key)

	log.Printf("[/storage/download] user %s is trying to download object, key: %s", currentUser, key)
	log.Printf("  uid: %s, username: %s, path: %s", uid, username, path)

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

	if status, msg, gate := checkAccess(obj, currentUser, pwd); status != 0 {
		log.Printf("  Access denied (%s), returning %d", gate, status)
		writeJSONError(w, status, msg, gate)
		return
	}

	log.Printf("  resp: username: %s, filename: %s, path: %s, uid: %s", obj.Username, obj.Filename, obj.Path, obj.ID)

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
	log.Printf("[/storage/remove] user %s is trying to remove objects, including key %s", username, keys)
	resp := api.RemoveResponse{Results: make(map[string]string)}
	for _, key := range keys {
		// First, remove from indexdb
		path, err := removeByID(username, key)
		if err != nil {
			resp.Results[key] = "error removing from indexdb: " + err.Error()
			log.Printf("  key: %s, path: %s; error removing from indexdb: %v", key, path, err)
			continue
		} else {
			log.Printf("  key: %s, path: %s; removed from indexdb", key, path)
		}

		// Then, remove from object storage
		err = opremove(r.Context(), path)
		if err != nil {
			resp.Results[key] = "error removing from object storage: " + err.Error()
			log.Printf("  key: %s, path: %s; error removing from object storage: %v", key, path, err)
			continue
		} else {
			log.Printf("  key: %s, path: %s; removed from object storage", key, path)
		}

		resp.Results[key] = "removed"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// uploadChunk handles one chunk of a multi-part chunked upload.
//
// When the object storage backend implements object.StreamingWriter (e.g. R2),
// each chunk is uploaded to object storage immediately as a multipart part.
// This avoids holding the final HTTP connection open while the server uploads
// the entire reassembled file, preventing Cloudflare 524 gateway timeouts.
//
// For backends that do not implement StreamingWriter (e.g. SQLite), the
// original behaviour is preserved: chunks are written to disk and the full
// file is assembled and uploaded when the last chunk arrives.
func uploadChunk(w http.ResponseWriter, r *http.Request, username string) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	uploadID := r.FormValue("upload_id")
	chunkIndexStr := r.FormValue("chunk_index")
	totalChunksStr := r.FormValue("total_chunks")
	if uploadID == "" || chunkIndexStr == "" || totalChunksStr == "" {
		http.Error(w, "missing upload_id, chunk_index, or total_chunks", http.StatusBadRequest)
		return
	}

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		http.Error(w, "invalid chunk_index", http.StatusBadRequest)
		return
	}
	totalChunks, err := strconv.Atoi(totalChunksStr)
	if err != nil {
		http.Error(w, "invalid total_chunks", http.StatusBadRequest)
		return
	}

	chunkFile, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer chunkFile.Close()

	_, supportsStreaming := objectStorage.(object.StreamingWriter)

	scope, err := normalizeScope(r.FormValue("access"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get or create the session on first chunk arrival.
	chunkSessionsMu.Lock()
	session, exists := chunkSessions[uploadID]
	if !exists {
		session = &chunkSession{
			total:     totalChunks,
			received:  make(map[int]bool),
			streaming: supportsStreaming,
			key:       r.FormValue("key"),
			path:      r.FormValue("path"),
			password:  r.FormValue("password"),
			force:     r.FormValue("force") == "true",
			meta:      r.FormValue("meta"),
			scope:     scope,
			username:  username,
		}
		if supportsStreaming {
			session.r2Parts = make(map[int32]object.CompletedPart)
		} else {
			tempDir, err := os.MkdirTemp("", "codesfer_chunk_"+uploadID)
			if err != nil {
				chunkSessionsMu.Unlock()
				http.Error(w, "failed to create temp dir: "+err.Error(), http.StatusInternalServerError)
				return
			}
			session.tempDir = tempDir
		}
		chunkSessions[uploadID] = session
	}
	chunkSessionsMu.Unlock()

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.streaming {
		sw := objectStorage.(object.StreamingWriter)

		// On the first chunk processed for this session, resolve the upload path
		// and create the R2 multipart upload.
		if session.r2UploadID == "" {
			path := session.path
			if path == "" || path == "." || path == "/" {
				path = uploadID
			}
			files, err := getFiles(session.username)
			if err != nil {
				http.Error(w, "failed to get existing files: "+err.Error(), http.StatusInternalServerError)
				return
			}
			idx := 1
			haveF, err := haveFile(session.username, path)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if haveF {
				for {
					conflict := false
					for _, f := range files {
						if f.Filename == fmt.Sprintf("%s_%d", path, idx) {
							conflict = true
						}
					}
					if !conflict {
						path = fmt.Sprintf("%s_%d", path, idx)
						log.Printf("[/storage/upload/chunk] streaming: rename complete: %s", path)
						break
					}
					idx++
				}
			}
			session.path = path
			session.r2ObjectPath = objPath(session.username, path)

			mUploadID, err := sw.CreateMultipart(r.Context(), session.r2ObjectPath, nil)
			if err != nil {
				http.Error(w, "failed to create multipart upload: "+err.Error(), http.StatusInternalServerError)
				return
			}
			session.r2UploadID = mUploadID
			log.Printf("[/storage/upload/chunk] streaming: created R2 multipart %s for %s", mUploadID, session.r2ObjectPath)
		}

		// Buffer chunk to a temp file to obtain its size for the UploadPart call.
		tmpFile, err := os.CreateTemp("", fmt.Sprintf("codesfer_part_%s_%d_*", uploadID, chunkIndex))
		if err != nil {
			http.Error(w, "failed to create temp file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tmpPath := tmpFile.Name()
		if _, err := io.Copy(tmpFile, chunkFile); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			http.Error(w, "failed to write chunk: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tmpFile.Close()

		info, err := os.Stat(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			http.Error(w, "failed to stat chunk: "+err.Error(), http.StatusInternalServerError)
			return
		}

		f, err := os.Open(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			http.Error(w, "failed to open chunk: "+err.Error(), http.StatusInternalServerError)
			return
		}

		partNumber := int32(chunkIndex + 1)
		etag, err := sw.UploadPart(r.Context(), session.r2ObjectPath, session.r2UploadID, partNumber, f, info.Size())
		f.Close()
		os.Remove(tmpPath)

		if err != nil {
			sw.AbortMultipart(context.Background(), session.r2ObjectPath, session.r2UploadID)
			http.Error(w, "failed to upload part: "+err.Error(), http.StatusInternalServerError)
			return
		}

		session.r2Parts[partNumber] = object.CompletedPart{ETag: etag, PartNumber: partNumber}
		session.received[chunkIndex] = true
		log.Printf("[/storage/upload/chunk] streaming: upload_id: %s, chunk %d/%d uploaded to R2", uploadID, chunkIndex+1, session.total)

		if len(session.received) < session.total {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		// All chunks are now in R2 — complete the multipart upload.
		chunkSessionsMu.Lock()
		delete(chunkSessions, uploadID)
		chunkSessionsMu.Unlock()

		parts := make([]object.CompletedPart, 0, len(session.r2Parts))
		for _, p := range session.r2Parts {
			parts = append(parts, p)
		}
		sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })

		if _, err := sw.CompleteMultipart(r.Context(), session.r2ObjectPath, session.r2UploadID, parts); err != nil {
			http.Error(w, "failed to complete multipart upload: "+err.Error(), http.StatusInternalServerError)
			return
		}

		key := session.key
		if key == "" {
			key, err = generateID(4)
			if err != nil {
				http.Error(w, "failed to generate id: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if session.force {
			err = upsert(key, session.username, session.path, session.password, session.r2ObjectPath, session.meta, session.scope)
		} else {
			err = insert(key, session.username, session.path, session.password, session.r2ObjectPath, session.meta, session.scope)
		}
		if err != nil {
			http.Error(w, "failed to save record: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.UploadResponse{Uid: key, Path: session.path})
		return
	}

	// === Temp-file fallback (non-streaming backends, e.g. SQLite) ===

	// Persist the chunk to disk.
	chunkPath := filepath.Join(session.tempDir, fmt.Sprintf("chunk_%d", chunkIndex))
	dst, err := os.Create(chunkPath)
	if err != nil {
		http.Error(w, "failed to create chunk file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(dst, chunkFile); err != nil {
		dst.Close()
		http.Error(w, "failed to write chunk: "+err.Error(), http.StatusInternalServerError)
		return
	}
	dst.Close()
	session.received[chunkIndex] = true

	log.Printf("[/storage/upload/chunk] upload_id: %s, chunk %d/%d received", uploadID, chunkIndex+1, session.total)

	if len(session.received) < session.total {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// All chunks received — remove from session map and assemble.
	chunkSessionsMu.Lock()
	delete(chunkSessions, uploadID)
	chunkSessionsMu.Unlock()

	log.Printf("[/storage/upload/chunk] upload_id: %s, assembling %d chunks", uploadID, session.total)

	// Calculate total assembled size.
	var totalSize int64
	for i := 0; i < session.total; i++ {
		info, err := os.Stat(filepath.Join(session.tempDir, fmt.Sprintf("chunk_%d", i)))
		if err != nil {
			os.RemoveAll(session.tempDir)
			http.Error(w, "chunk missing: "+err.Error(), http.StatusInternalServerError)
			return
		}
		totalSize += info.Size()
	}

	// Stream chunks in order via a pipe so opupload receives a plain io.Reader.
	pr, pw := io.Pipe()
	go func() {
		defer os.RemoveAll(session.tempDir)
		for i := 0; i < session.total; i++ {
			f, err := os.Open(filepath.Join(session.tempDir, fmt.Sprintf("chunk_%d", i)))
			if err != nil {
				pw.CloseWithError(err)
				return
			}
			_, copyErr := io.Copy(pw, f)
			f.Close()
			if copyErr != nil {
				pw.CloseWithError(copyErr)
				return
			}
		}
		pw.Close()
	}()

	// Resolve upload path (same conflict-detection logic as upload()).
	path := session.path
	if path == "" || path == "." || path == "/" {
		path = uploadID
	}

	files, err := getFiles(session.username)
	if err != nil {
		pr.Close()
		http.Error(w, "failed to get existing files: "+err.Error(), http.StatusInternalServerError)
		return
	}
	idx := 1
	haveF, err := haveFile(session.username, path)
	if err != nil {
		pr.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if haveF {
		for {
			conflict := false
			for _, f := range files {
				if f.Filename == fmt.Sprintf("%s_%d", path, idx) {
					conflict = true
					log.Printf("[/storage/upload/chunk] path conflict, trying: %s", fmt.Sprintf("%s_%d", path, idx))
				}
			}
			if !conflict {
				path = fmt.Sprintf("%s_%d", path, idx)
				log.Printf("[/storage/upload/chunk] rename complete: %s", path)
				break
			}
			idx++
		}
	}

	uid, err := opupload(r.Context(), pr, totalSize, session.key, session.username, session.password, path, session.force, session.meta, session.scope)
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

// info returns metadata about a code snippet without downloading it
// key: <uid> || <username>/<uid> || <username>/<path>
func info(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	pwd := r.URL.Query().Get("password")
	currentUser := r.Header.Get("X-Username")
	uid, username, path := parseKey(key) // Parse key - same logic as download()

	log.Printf("[/storage/info] user %s is inspecting object, key: %s", currentUser, key)
	log.Printf("  uid: %s, username: %s, path: %s", uid, username, path)

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
		Path:        obj.Filename,
		CreatedAt:   obj.CreatedAt,
		Protected:   obj.Password != "",
		AccessScope: obj.AccessScope,
		Metadata:    metadata,
	})
}
