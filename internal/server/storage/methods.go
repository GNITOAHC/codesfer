package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"codesfer/pkg/object"
)

func RemoveOrphanedObject() error {
	log.Println("Removing orphaned objects...")
	indexed, err := getAllPaths()
	if err != nil {
		return err
	}
	objects, err := objectStorage.List(context.Background(), "")
	if err != nil {
		return err
	}

	// Create a map for O(1) lookup of indexed paths
	indexedMap := make(map[string]struct{}, len(indexed))
	for _, path := range indexed {
		log.Printf("Indexed path: %s", path)
		indexedMap[path] = struct{}{}
	}

	var orphans []string
	for _, obj := range objects {
		log.Printf("Object: %s", obj.Key)
		if _, exists := indexedMap[obj.Key]; !exists {
			orphans = append(orphans, obj.Key)
		}
	}

	log.Printf("Found %d orphaned objects", len(orphans))
	for _, orphan := range orphans {
		log.Printf("Removing orphaned object: %s", orphan)
		if err := objectStorage.Delete(context.Background(), orphan); err != nil {
			log.Printf("Failed to remove orphaned object %s: %v", orphan, err)
		}
	}

	return nil
}

// DownloadRoute is the route for downloading an object
//
//	curl -OL http://base_url/download/<key>.zip
//	wget http://base_url/download/<key>.zip
func DownloadRoute(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimRight(r.PathValue("key"), ".zip")
	log.Printf("[/download] anonymous user is trying to download object, key: %s", key)

	var obj *Object
	var err error
	if obj, err = get(key); obj != nil || err != nil {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("  Object found by key: %s", obj.ID)
	} else {
		http.Error(w, "object not found", http.StatusNotFound)
		return
	}

	if obj.Password != "" {
		http.Error(w, "object is password protected, use CLI to download", http.StatusUnauthorized)
		return
	}

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

	filename := sanitizeFilename(obj.Path)
	if !strings.HasSuffix(filename, ".zip") {
		filename += ".zip"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
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
