package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gnitoahc/codesfer/pkg/api"
	"github.com/gnitoahc/codesfer/pkg/object"
	"github.com/gnitoahc/codesfer/pkg/sqlite"
)

// setupChunkRoutes wires an index db and a sqlite object backend behind
// POST /upload/chunk and GET /download. The auth middleware is not in play, so
// tests set X-Username themselves.
func setupChunkRoutes(t *testing.T) *httptest.Server {
	t.Helper()
	setupIndexDB(t)

	backend := &sqlite.Storage{}
	source := fmt.Sprintf("file:%s?cache=shared&mode=rwc", filepath.Join(t.TempDir(), "objects.db"))
	if err := backend.Init(context.Background(), sqlite.Config{Source: source, Table: "objects", AllowOverwrite: true}); err != nil {
		t.Fatalf("init object storage: %v", err)
	}
	objectStorage = backend

	chunkSessionsMu.Lock()
	chunkSessions = map[string]*chunkSession{}
	chunkSessionsMu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /upload/chunk", requireUser("upload", uploadChunk))
	mux.HandleFunc("GET /download", download)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func postChunk(t *testing.T, srv *httptest.Server, username, uploadID, path string, index, total int, data []byte) *http.Response {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"upload_id":    uploadID,
		"chunk_index":  strconv.Itoa(index),
		"total_chunks": strconv.Itoa(total),
		"path":         path,
	} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	part, err := writer.CreateFormFile("file", fmt.Sprintf("chunk_%d", index))
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write chunk body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req, _ := http.NewRequest("POST", srv.URL+"/upload/chunk", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Username", username)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func TestUploadChunkAssembles(t *testing.T) {
	srv := setupChunkRoutes(t)
	chunks := [][]byte{[]byte("alpha"), []byte("beta"), []byte("gamma")}

	for i := range chunks[:2] {
		resp := postChunk(t, srv, "alice", "up1", "notes.zip", i, len(chunks), chunks[i])
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("chunk %d: got %d want 202", i, resp.StatusCode)
		}
	}

	resp := postChunk(t, srv, "alice", "up1", "notes.zip", 2, len(chunks), chunks[2])
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("last chunk: got %d want 200", resp.StatusCode)
	}
	var result api.UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Path != "notes.zip" || result.Uid == "" {
		t.Fatalf("response = %+v, want path notes.zip and a uid", result)
	}

	// The session must be gone, and the object downloadable in chunk order.
	chunkSessionsMu.Lock()
	remaining := len(chunkSessions)
	chunkSessionsMu.Unlock()
	if remaining != 0 {
		t.Fatalf("%d session(s) left after completion", remaining)
	}

	dl, err := http.Get(srv.URL + "/download?key=" + result.Uid)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer dl.Body.Close()
	got, _ := io.ReadAll(dl.Body)
	if string(got) != "alphabetagamma" {
		t.Fatalf("downloaded %q, want %q", got, "alphabetagamma")
	}
}

// A second upload of the same filename must not clobber the first.
func TestUploadChunkRenamesConflict(t *testing.T) {
	srv := setupChunkRoutes(t)

	for _, id := range []string{"up1", "up2"} {
		resp := postChunk(t, srv, "alice", id, "notes.zip", 0, 1, []byte("x"))
		var result api.UploadResponse
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: got %d want 200", id, resp.StatusCode)
		}
		want := map[string]string{"up1": "notes.zip", "up2": "notes.zip_1"}[id]
		if result.Path != want {
			t.Fatalf("%s: path = %q want %q", id, result.Path, want)
		}
	}
}

// The upload id is client-generated, so another user must not be able to push
// chunks into an in-progress upload.
func TestUploadChunkRejectsOtherUser(t *testing.T) {
	srv := setupChunkRoutes(t)

	resp := postChunk(t, srv, "alice", "up1", "notes.zip", 0, 2, []byte("alpha"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("alice chunk: got %d want 202", resp.StatusCode)
	}

	resp = postChunk(t, srv, "mallory", "up1", "notes.zip", 1, 2, []byte("evil"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("mallory chunk: got %d want 403", resp.StatusCode)
	}
}

// expireSession rewinds a session's idle clock so a sweep sees it as stale,
// without the test having to sleep.
func expireSession(t *testing.T, uploadID string) {
	t.Helper()
	chunkSessionsMu.Lock()
	defer chunkSessionsMu.Unlock()
	session, ok := chunkSessions[uploadID]
	if !ok {
		t.Fatalf("no session for upload_id %s", uploadID)
	}
	session.lastSeen = time.Now().Add(-24 * time.Hour)
}

func liveSessions() int {
	chunkSessionsMu.Lock()
	defer chunkSessionsMu.Unlock()
	return len(chunkSessions)
}

func TestSweepStaleChunkSessions(t *testing.T) {
	srv := setupChunkRoutes(t)

	resp := postChunk(t, srv, "alice", "up1", "notes.zip", 0, 2, []byte("alpha"))
	resp.Body.Close()

	// Recently touched: must survive even a sweep running back to back.
	for range 100 {
		if err := SweepStaleChunkSessions(context.Background(), time.Hour); err != nil {
			t.Fatalf("sweep: %v", err)
		}
	}
	if n := liveSessions(); n != 1 {
		t.Fatalf("fresh session swept: %d left, want 1", n)
	}

	expireSession(t, "up1")
	if err := SweepStaleChunkSessions(context.Background(), time.Hour); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n := liveSessions(); n != 0 {
		t.Fatalf("stale session kept: %d left, want 0", n)
	}
}

// The TTL must measure the gap between chunks, not the age of the upload, so
// an upload of any duration survives as long as it keeps making progress.
func TestChunkResetsIdleClock(t *testing.T) {
	srv := setupChunkRoutes(t)

	resp := postChunk(t, srv, "alice", "up1", "notes.zip", 0, 3, []byte("alpha"))
	resp.Body.Close()
	expireSession(t, "up1") // pretend a long time passed before the next chunk

	resp = postChunk(t, srv, "alice", "up1", "notes.zip", 1, 3, []byte("beta"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("second chunk: got %d want 202", resp.StatusCode)
	}

	if err := SweepStaleChunkSessions(context.Background(), time.Hour); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n := liveSessions(); n != 1 {
		t.Fatalf("session swept despite recent chunk: %d left, want 1", n)
	}
}

// blockingBackend parks inside UploadPart so a test can sweep while a request
// is provably mid-flight. release is buffered and idempotent so a failing
// assertion unblocks the parked request instead of hanging the suite.
type blockingBackend struct {
	object.ObjectStorage
	entered chan struct{}
	release chan struct{}
	unblock func()
}

func newBlockingBackend(t *testing.T, inner object.ObjectStorage) *blockingBackend {
	t.Helper()
	b := &blockingBackend{
		ObjectStorage: inner,
		entered:       make(chan struct{}, 1),
		release:       make(chan struct{}),
	}
	b.unblock = sync.OnceFunc(func() { close(b.release) })
	t.Cleanup(b.unblock)
	return b
}

func (b *blockingBackend) UploadPart(ctx context.Context, key, uploadID string, partNumber int32, body io.Reader, size int64) (string, error) {
	b.entered <- struct{}{}
	<-b.release
	return b.ObjectStorage.UploadPart(ctx, key, uploadID, partNumber, body, size)
}

// A request holding a session must never have its multipart aborted underneath
// it, however aggressively the sweep runs.
func TestSweepSkipsInFlightSession(t *testing.T) {
	srv := setupChunkRoutes(t)
	blocker := newBlockingBackend(t, objectStorage)
	objectStorage = blocker

	done := make(chan int, 1)
	go func() {
		resp := postChunk(t, srv, "alice", "up1", "notes.zip", 0, 2, []byte("alpha"))
		resp.Body.Close()
		done <- resp.StatusCode
	}()

	<-blocker.entered // request is inside UploadPart, holding the session

	expireSession(t, "up1")
	for range 100 { // as if the cron fired every instant
		if err := SweepStaleChunkSessions(context.Background(), time.Hour); err != nil {
			t.Fatalf("sweep: %v", err)
		}
	}
	if n := liveSessions(); n != 1 {
		t.Fatalf("in-flight session swept: %d left, want 1", n)
	}

	blocker.unblock()
	if status := <-done; status != http.StatusAccepted {
		t.Fatalf("in-flight chunk: got %d want 202", status)
	}
}

// A whole upload must complete while the sweep hammers away in parallel.
// Run with -race to also cover the shared session fields.
func TestUploadChunkSurvivesConcurrentSweep(t *testing.T) {
	srv := setupChunkRoutes(t)

	stop := make(chan struct{})
	swept := make(chan struct{})
	go func() {
		defer close(swept)
		for {
			select {
			case <-stop:
				return
			default:
				if err := SweepStaleChunkSessions(context.Background(), time.Minute); err != nil {
					t.Errorf("sweep: %v", err)
					return
				}
			}
		}
	}()

	chunks := [][]byte{[]byte("alpha"), []byte("beta"), []byte("gamma")}
	var last *http.Response
	for i := range chunks {
		resp := postChunk(t, srv, "alice", "up1", "notes.zip", i, len(chunks), chunks[i])
		if i < len(chunks)-1 {
			resp.Body.Close()
			if resp.StatusCode != http.StatusAccepted {
				t.Fatalf("chunk %d: got %d want 202", i, resp.StatusCode)
			}
			continue
		}
		last = resp
	}
	close(stop)
	<-swept

	defer last.Body.Close()
	if last.StatusCode != http.StatusOK {
		t.Fatalf("last chunk: got %d want 200", last.StatusCode)
	}
	var result api.UploadResponse
	if err := json.NewDecoder(last.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	dl, err := http.Get(srv.URL + "/download?key=" + result.Uid)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer dl.Body.Close()
	got, _ := io.ReadAll(dl.Body)
	if string(got) != "alphabetagamma" {
		t.Fatalf("downloaded %q, want %q", got, "alphabetagamma")
	}
}

// Resuming into a session that no longer exists can never reach total_chunks,
// so it must fail loudly instead of stalling at 202 forever.
func TestUploadChunkRejectsExpiredSession(t *testing.T) {
	srv := setupChunkRoutes(t)

	resp := postChunk(t, srv, "alice", "up1", "notes.zip", 0, 3, []byte("alpha"))
	resp.Body.Close()

	expireSession(t, "up1")
	if err := SweepStaleChunkSessions(context.Background(), time.Hour); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	resp = postChunk(t, srv, "alice", "up1", "notes.zip", 1, 3, []byte("beta"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("chunk after expiry: got %d want 410", resp.StatusCode)
	}
}

func TestUploadChunkRejectsTotalMismatch(t *testing.T) {
	srv := setupChunkRoutes(t)

	resp := postChunk(t, srv, "alice", "up1", "notes.zip", 0, 3, []byte("alpha"))
	resp.Body.Close()

	resp = postChunk(t, srv, "alice", "up1", "notes.zip", 1, 5, []byte("beta"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("total_chunks mismatch: got %d want 400", resp.StatusCode)
	}
}

func TestUploadChunkRejectsBadIndex(t *testing.T) {
	srv := setupChunkRoutes(t)

	resp := postChunk(t, srv, "alice", "up1", "notes.zip", 3, 2, []byte("x"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("out-of-range chunk_index: got %d want 400", resp.StatusCode)
	}
}
