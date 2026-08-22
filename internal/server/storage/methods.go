package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
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

	obj, err := get(key)
	if err != nil {
		log.Printf("  index lookup error: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error", "")
		return
	}
	if obj == nil {
		writeJSONError(w, http.StatusNotFound, "not found", "")
		return
	}
	log.Printf("  Object found by key: %s", obj.ID)

	if servePreview(w, r, obj) {
		return
	}

	// This route treats every viewer as anonymous: public objects gate on
	// password only, scoped objects always ask for login (use /storage/download
	// for authenticated access).
	if status, msg, gate := checkAccess(obj, "", r.URL.Query().Get("password")); status != 0 {
		writeJSONError(w, status, msg, gate)
		return
	}

	meta, body, err := objectStorage.Get(r.Context(), obj.ObjPath, nil)
	if err != nil {
		log.Printf("  object storage read error: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error", "")
		return
	}
	defer body.Close()

	filename := sanitizeFilename(obj.ObjPath)
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

func servePreview(w http.ResponseWriter, r *http.Request, obj *Object) bool {
	if !isPreviewBot(r.Header.Get("User-Agent")) {
		return false
	}
	log.Printf("  Preview bot detected: %s", r.Header.Get("User-Agent"))
	meta, err := objectStorage.Stat(r.Context(), obj.ObjPath)
	if err != nil {
		// If we can't get stats, we can still show basic preview or just error out.
		// Choosing to log and show basic info if possible, or error.
		log.Printf("Failed to stat object for preview: %v", err)
	}

	filename := sanitizeFilename(obj.ObjPath)
	if obj.IdxPath != "" {
		filename = obj.IdxPath
	}

	description := "Ready to download"
	if meta.Size > 0 {
		description = fmt.Sprintf("Size: %s", formatSize(meta.Size))
	}
	if obj.Password != "" {
		description = "🔒 Password Protected"
		if meta.Size > 0 {
			description += fmt.Sprintf(" • Size: %s", formatSize(meta.Size))
		}
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>%s</title>
    <meta property="og:title" content="%s">
    <meta property="og:description" content="%s">
    <meta property="og:type" content="website">
    <meta name="theme-color" content="#2b2d31">
</head>
<body>
    <p>%s</p>
    <p>%s</p>
</body>
</html>`, filename, filename, description, filename, description)
	return true
}

func isPreviewBot(userAgent string) bool {
	bots := []string{
		"Discordbot",
		"facebookexternalhit",
		"Twitterbot",
		"WhatsApp",
		"TelegramBot",
		"Slackbot",
		"SkypeUriPreview",
	}
	for _, bot := range bots {
		if strings.Contains(userAgent, bot) {
			return true
		}
	}
	return false
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
