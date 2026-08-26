package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gnitoahc/codesfer/pkg/api"
	"github.com/gnitoahc/codesfer/pkg/object"
)

// chunkSession tracks an in-progress chunked upload. Every chunk is forwarded
// to object storage as a multipart part on arrival, so the server never holds
// the whole file — that is what keeps large uploads under Cloudflare's 524
// gateway timeout.
//
// Locking: chunkSessionsMu guards the session map plus the lifetime fields
// (lastSeen, inFlight); mu serialises the request-owned fields (received,
// parts) between concurrent chunks of the same upload. A handler holding mu
// may take chunkSessionsMu, never the reverse.
type chunkSession struct {
	mu       sync.Mutex
	total    int
	received map[int]bool
	parts    map[int32]object.CompletedPart

	uploadID string // multipart upload id from the backend
	objPath  string

	// lastSeen is when a request last touched this session; inFlight counts
	// the requests using it right now. The sweep skips a session unless both
	// say it is idle, so an upload in progress can never be abandoned no
	// matter how often the sweep runs. Both guarded by chunkSessionsMu.
	lastSeen time.Time
	inFlight int

	username string
	key      string
	path     string
	password string
	meta     string
	scope    string
	force    bool
}

var (
	chunkSessions   = make(map[string]*chunkSession)
	chunkSessionsMu sync.Mutex
)

// uploadChunk handles one chunk of a multi-part chunked upload. Chunks may
// arrive in any order; the last one to arrive completes the upload and writes
// the index record.
func uploadChunk(w http.ResponseWriter, r *http.Request, username string) {
	if err := r.ParseMultipartForm(maxUploadMemory); err != nil {
		http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	uploadID := r.FormValue("upload_id")
	chunkIndex, err := strconv.Atoi(r.FormValue("chunk_index"))
	if err != nil {
		http.Error(w, "invalid or missing chunk_index", http.StatusBadRequest)
		return
	}
	totalChunks, err := strconv.Atoi(r.FormValue("total_chunks"))
	if err != nil {
		http.Error(w, "invalid or missing total_chunks", http.StatusBadRequest)
		return
	}
	if uploadID == "" || totalChunks < 1 || chunkIndex < 0 || chunkIndex >= totalChunks {
		http.Error(w, "missing upload_id or chunk index out of range", http.StatusBadRequest)
		return
	}

	scope, err := normalizeScope(r.FormValue("access"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	chunkFile, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer chunkFile.Close()

	session, err := acquireSession(r.Context(), uploadID, username, chunkIndex, totalChunks, scope, r)
	if err != nil {
		writeSessionError(w, err)
		return
	}
	defer releaseSession(session)

	session.mu.Lock()
	defer session.mu.Unlock()

	partNumber := int32(chunkIndex + 1)
	etag, err := uploadPart(r.Context(), session, partNumber, chunkFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	session.parts[partNumber] = object.CompletedPart{ETag: etag, PartNumber: partNumber}
	session.received[chunkIndex] = true
	log.Printf("[/storage/upload/chunk] upload_id: %s, chunk %d/%d uploaded", uploadID, chunkIndex+1, session.total)

	if len(session.received) < session.total {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Last chunk: finalise the object, then record it.
	chunkSessionsMu.Lock()
	delete(chunkSessions, uploadID)
	chunkSessionsMu.Unlock()

	parts := make([]object.CompletedPart, 0, len(session.parts))
	for _, p := range session.parts {
		parts = append(parts, p)
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })

	if _, err := objectStorage.CompleteMultipart(r.Context(), session.objPath, session.uploadID, parts); err != nil {
		// The session is out of the map, so the sweep will never see this
		// multipart again — release it here or it leaks.
		if abortErr := objectStorage.AbortMultipart(context.WithoutCancel(r.Context()), session.objPath, session.uploadID); abortErr != nil {
			log.Printf("[/storage/upload/chunk] abort after failed complete: %v", abortErr)
		}
		http.Error(w, "failed to complete multipart upload: "+err.Error(), http.StatusInternalServerError)
		return
	}

	key, err := saveRecord(session.key, session.username, session.path, session.password, session.objPath, session.meta, session.scope, session.force)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.UploadResponse{Uid: key, Path: session.path})
}

// sessionError carries the HTTP status a session problem should surface as.
type sessionError struct {
	status int
	msg    string
}

func (e sessionError) Error() string { return e.msg }

func writeSessionError(w http.ResponseWriter, err error) {
	var se sessionError
	if errors.As(err, &se) {
		http.Error(w, se.msg, se.status)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// acquireSession returns the session for uploadID, starting a backend
// multipart upload the first time it is seen, and marks it in use so the sweep
// cannot abandon it while the request runs. Every successful call must be
// paired with releaseSession.
//
// the global lock is held across the CreateMultipart round-trip, so concurrent
// first-chunks serialise on it. Move to a per-uploadID lock if that ever shows up in latency.
func acquireSession(ctx context.Context, uploadID, username string, chunkIndex, totalChunks int, scope string, r *http.Request) (*chunkSession, error) {
	chunkSessionsMu.Lock()
	defer chunkSessionsMu.Unlock()

	if session, ok := chunkSessions[uploadID]; ok {
		// The upload id is client-generated, so a session must only ever
		// accept chunks from the user who started it.
		if session.username != username {
			return nil, sessionError{http.StatusForbidden, "upload_id belongs to another user"}
		}
		if session.total != totalChunks {
			return nil, sessionError{http.StatusBadRequest, fmt.Sprintf("total_chunks changed mid-upload: session has %d, got %d", session.total, totalChunks)}
		}
		session.inFlight++
		return session, nil
	}

	// No session: either this is the first chunk, or an earlier one expired.
	// Starting fresh half way through would silently produce an upload that
	// can never reach total_chunks, so say so instead.
	if chunkIndex != 0 {
		return nil, sessionError{http.StatusGone, "upload session expired or unknown; restart the upload"}
	}

	path := r.FormValue("path")
	if path == "" || path == "." || path == "/" {
		path = uploadID
	}
	path, err := uniqueIdxPath(username, path)
	if err != nil {
		return nil, err
	}

	session := &chunkSession{
		total:    totalChunks,
		received: make(map[int]bool),
		parts:    make(map[int32]object.CompletedPart),
		objPath:  objPath(username, path),
		lastSeen: time.Now(),
		inFlight: 1,
		username: username,
		key:      r.FormValue("key"),
		path:     path,
		password: r.FormValue("password"),
		meta:     r.FormValue("meta"),
		scope:    scope,
		force:    r.FormValue("force") == "true",
	}

	session.uploadID, err = objectStorage.CreateMultipart(ctx, session.objPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart upload: %w", err)
	}
	log.Printf("[/storage/upload/chunk] created multipart %s for %s", session.uploadID, session.objPath)

	chunkSessions[uploadID] = session
	return session, nil
}

// releaseSession marks the request done and resets the idle clock, so the TTL
// measures the gap between chunks rather than the age of the upload.
func releaseSession(session *chunkSession) {
	chunkSessionsMu.Lock()
	defer chunkSessionsMu.Unlock()
	session.inFlight--
	session.lastSeen = time.Now()
}

// uploadPart forwards one chunk to object storage. The chunk is buffered to a
// temp file first because UploadPart needs its size up front.
func uploadPart(ctx context.Context, session *chunkSession, partNumber int32, body io.Reader) (string, error) {
	tmp, err := os.CreateTemp("", "codesfer_part_*")
	if err != nil {
		return "", fmt.Errorf("failed to buffer chunk: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	size, err := io.Copy(tmp, body)
	if err != nil {
		return "", fmt.Errorf("failed to write chunk: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("failed to rewind chunk: %w", err)
	}

	etag, err := objectStorage.UploadPart(ctx, session.objPath, session.uploadID, partNumber, tmp, size)
	if err != nil {
		return "", fmt.Errorf("failed to upload part: %w", err)
	}
	return etag, nil
}

// SweepStaleChunkSessions abandons uploads left untouched for longer than ttl
// and releases their backend multipart uploads. This also cleans up after
// requests that failed part-way through.
//
// Safe to run at any interval: sessions with a request in flight are skipped,
// and the ones taken are removed from the map before being aborted, so no
// handler can be using a multipart that this function cancels.
func SweepStaleChunkSessions(ctx context.Context, ttl time.Duration) error {
	cutoff := time.Now().Add(-ttl)

	chunkSessionsMu.Lock()
	stale := map[string]*chunkSession{}
	for id, session := range chunkSessions {
		if session.inFlight > 0 || !session.lastSeen.Before(cutoff) {
			continue
		}
		stale[id] = session
		delete(chunkSessions, id)
	}
	chunkSessionsMu.Unlock()

	for id, session := range stale {
		log.Printf("[chunk sweep] aborting stale upload %s (%s)", id, session.objPath)
		if err := objectStorage.AbortMultipart(ctx, session.objPath, session.uploadID); err != nil {
			log.Printf("[chunk sweep] abort %s failed: %v", id, err)
		}
	}
	return nil
}
