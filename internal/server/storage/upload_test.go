package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gnitoahc/codesfer/internal/constants"
	"github.com/gnitoahc/codesfer/pkg/api"
	"github.com/gnitoahc/codesfer/pkg/object"
	"github.com/gnitoahc/codesfer/pkg/object/sqlite"
)

// setupUploadRoutes wires an index db and a sqlite object backend behind
// POST /upload, POST /upload/chunk and GET /download, with a clean anonymous
// rate limiter. Tests set X-Username themselves; empty means anonymous.
func setupUploadRoutes(t *testing.T) *httptest.Server {
	t.Helper()
	setupIndexDB(t)

	backend := &sqlite.Storage{}
	source := fmt.Sprintf("file:%s?cache=shared&mode=rwc", filepath.Join(t.TempDir(), "objects.db"))
	if err := backend.Init(context.Background(), sqlite.Config{Source: source, Table: "objects"}); err != nil {
		t.Fatalf("init object storage: %v", err)
	}
	objectStorage = backend

	chunkSessionsMu.Lock()
	chunkSessions = map[string]*chunkSession{}
	chunkSessionsMu.Unlock()
	anonWindowsMu.Lock()
	anonWindows = map[string]*anonWindow{}
	anonWindowsMu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /upload", upload)
	mux.HandleFunc("POST /upload/chunk", requireUser("upload", uploadChunk))
	mux.HandleFunc("GET /download", download)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// postUpload sends a single-shot upload from ip with the given form fields.
func postUpload(t *testing.T, srv *httptest.Server, username, ip string, fields map[string]string, data []byte) *http.Response {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	part, err := writer.CreateFormFile("file", "notes.zip")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write file body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req, _ := http.NewRequest("POST", srv.URL+"/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Username", username)
	req.Header.Set("CF-Connecting-IP", ip)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func decodeUpload(t *testing.T, resp *http.Response) api.UploadResponse {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload: got %d want 200: %s", resp.StatusCode, msg)
	}
	var result api.UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

func storedObjects(t *testing.T, prefix string) int {
	t.Helper()
	objs, err := objectStorage.List(context.Background(), prefix)
	if err != nil {
		t.Fatalf("list objects: %v", err)
	}
	return len(objs)
}

// failingPut rejects every Put, as a backend outage would.
type failingPut struct{ object.ObjectStorage }

func (failingPut) Put(context.Context, string, io.Reader, int64, string, map[string]string) (object.Object, error) {
	return object.Object{}, errors.New("backend down")
}

// Bytes go first: if they never land, no record may point at them.
func TestUploadPutFailureLeavesNoRecord(t *testing.T) {
	srv := setupUploadRoutes(t)
	objectStorage = failingPut{objectStorage}

	resp := postUpload(t, srv, "alice", "", nil, []byte("zip-bytes"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("got %d want 500", resp.StatusCode)
	}
	if objs, err := show("alice"); err != nil || len(objs) != 0 {
		t.Fatalf("records after failed put: %+v %v", objs, err)
	}
}

// If the record cannot be written, the upload's own object is removed.
func TestUploadRecordConflictDeletesObject(t *testing.T) {
	srv := setupUploadRoutes(t)
	seedObject(t, "taken", "bob", "", ScopePublic, true)

	resp := postUpload(t, srv, "alice", "", map[string]string{"key": "taken"}, []byte("zip-bytes"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("got %d want 409", resp.StatusCode)
	}
	if n := storedObjects(t, "alice/"); n != 0 {
		t.Fatalf("%d object(s) left behind for alice", n)
	}
	if n := storedObjects(t, "bob/"); n != 1 {
		t.Fatalf("bob's object touched: %d left", n)
	}
}

// racingComplete records a concurrent upload of the same filename just
// before the multipart completes, so the final record write conflicts.
type racingComplete struct {
	object.ObjectStorage
	t *testing.T
}

func (b racingComplete) CompleteMultipart(ctx context.Context, key, uploadID string, parts []object.CompletedPart) (object.Object, error) {
	if err := insert("racer", "alice", "notes.zip", "", "alice/racer/notes.zip", "{}", ScopePublic, 0); err != nil {
		b.t.Fatalf("insert racer: %v", err)
	}
	return b.ObjectStorage.CompleteMultipart(ctx, key, uploadID, parts)
}

func TestUploadChunkRecordConflictDeletesObject(t *testing.T) {
	srv := setupUploadRoutes(t)
	objectStorage = racingComplete{objectStorage, t}

	resp := postChunk(t, srv, "alice", "up1", "notes.zip", 0, 1, []byte("alpha"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("got %d want 409", resp.StatusCode)
	}
	if n := storedObjects(t, "alice/"); n != 0 {
		t.Fatalf("%d object(s) left behind for alice", n)
	}
}

func TestAnonymousUpload(t *testing.T) {
	srv := setupUploadRoutes(t)

	// key, force and access must all be ignored for anonymous uploads.
	fields := map[string]string{"key": "custom", "force": "true", "access": ScopeOwner}
	first := decodeUpload(t, postUpload(t, srv, "", "1.2.3.4", fields, []byte("first")))
	second := decodeUpload(t, postUpload(t, srv, "", "1.2.3.4", fields, []byte("second")))

	for _, res := range []api.UploadResponse{first, second} {
		if len(res.Uid) != anonKeyLen || res.Uid == "custom" {
			t.Fatalf("uid = %q, want a generated %d-char key", res.Uid, anonKeyLen)
		}
		if !strings.HasSuffix(res.Path, "/notes.zip") {
			t.Fatalf("path = %q, want <id>/notes.zip", res.Path)
		}
		ttl := time.Until(time.Unix(res.ExpiresAt, 0))
		if ttl <= constants.AnonUploadTTL-time.Minute || ttl > constants.AnonUploadTTL {
			t.Fatalf("expires in %v, want about %v", ttl, constants.AnonUploadTTL)
		}
		obj, err := get(res.Uid)
		if err != nil || obj == nil || obj.Username != anonUsername || obj.AccessScope != ScopePublic {
			t.Fatalf("record: %+v %v", obj, err)
		}
	}
	if first.Uid == second.Uid || first.Path == second.Path {
		t.Fatalf("second anonymous upload collided with the first: %+v %+v", first, second)
	}

	// Both stay downloadable by anyone: the second did not replace the first.
	for res, want := range map[api.UploadResponse]string{first: "first", second: "second"} {
		dl, err := http.Get(srv.URL + "/download?key=" + res.Uid)
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		got, _ := io.ReadAll(dl.Body)
		dl.Body.Close()
		if string(got) != want {
			t.Fatalf("downloaded %q, want %q", got, want)
		}
	}
}

func TestAnonymousUploadSizeCap(t *testing.T) {
	srv := setupUploadRoutes(t)

	resp := postUpload(t, srv, "", "1.2.3.4", nil, make([]byte, constants.UploadChunkSize+anonFormSlack+1))
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d want 413", resp.StatusCode)
	}
	if n := storedObjects(t, ""); n != 0 {
		t.Fatalf("%d object(s) stored for an oversized upload", n)
	}
}

func TestAnonymousUploadRateLimit(t *testing.T) {
	srv := setupUploadRoutes(t)

	for i := range constants.AnonUploadLimit {
		resp := postUpload(t, srv, "", "1.2.3.4", nil, []byte("x"))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("upload %d: got %d want 200", i+1, resp.StatusCode)
		}
	}
	resp := postUpload(t, srv, "", "1.2.3.4", nil, []byte("x"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("over the limit: got %d want 429", resp.StatusCode)
	}

	resp = postUpload(t, srv, "", "5.6.7.8", nil, []byte("x"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("other ip: got %d want 200", resp.StatusCode)
	}

	// Signed-in uploads are not counted against the IP.
	resp = postUpload(t, srv, "alice", "1.2.3.4", nil, []byte("x"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signed-in upload: got %d want 200", resp.StatusCode)
	}

	PruneAnonWindows(time.Now().Add(constants.AnonUploadWindow))
	if !allowAnonUpload("1.2.3.4", time.Now()) {
		t.Fatal("limit not reset after the window ended")
	}
}

func TestAnonymousChunkUploadRejected(t *testing.T) {
	srv := setupUploadRoutes(t)

	resp := postChunk(t, srv, "", "up1", "notes.zip", 0, 1, []byte("x"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d want 401", resp.StatusCode)
	}
}

func TestExpiredObjects(t *testing.T) {
	srv := setupUploadRoutes(t)
	ctx := context.Background()

	past := time.Now().Add(-time.Minute).Unix()
	if err := insert("old", anonUsername, "x/old.zip", "", "anon/x/old.zip", "{}", ScopePublic, past); err != nil {
		t.Fatalf("insert expired: %v", err)
	}
	if _, err := objectStorage.Put(ctx, "anon/x/old.zip", strings.NewReader("old"), 3, "", nil); err != nil {
		t.Fatalf("put expired: %v", err)
	}
	seedObject(t, "keep", "alice", "", ScopePublic, true) // never expires

	// Expired reads as missing straight away, before any sweep.
	for _, key := range []string{"old", "anon/x/old.zip"} {
		dl, err := http.Get(srv.URL + "/download?key=" + key)
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		dl.Body.Close()
		if dl.StatusCode != http.StatusNotFound {
			t.Fatalf("download %s: got %d want 404", key, dl.StatusCode)
		}
	}

	if err := SweepExpiredObjects(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if taken, err := keyExists("old"); err != nil || taken {
		t.Fatalf("expired record kept: %v %v", taken, err)
	}
	if n := storedObjects(t, "anon/"); n != 0 {
		t.Fatalf("expired object kept: %d left", n)
	}
	if obj, err := get("keep"); err != nil || obj == nil {
		t.Fatalf("non-expiring record swept: %+v %v", obj, err)
	}
}
