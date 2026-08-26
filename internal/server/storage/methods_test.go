package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnitoahc/codesfer/pkg/object/sqlite"
)

// setupDownloadRoute wires an index db and a sqlite object storage backend,
// then returns a server exposing GET /download/{key}.
func setupDownloadRoute(t *testing.T) *httptest.Server {
	t.Helper()
	setupIndexDB(t)

	backend := &sqlite.Storage{}
	source := fmt.Sprintf("file:%s?cache=shared&mode=rwc", filepath.Join(t.TempDir(), "objects.db"))
	if err := backend.Init(context.Background(), sqlite.Config{Source: source, Table: "objects"}); err != nil {
		t.Fatalf("init object storage: %v", err)
	}
	objectStorage = backend

	mux := http.NewServeMux()
	mux.HandleFunc("GET /download/{key}", DownloadRoute)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func seedObject(t *testing.T, key, owner, password, scope string, storeBody bool) {
	t.Helper()
	path := owner + "/" + key + ".zip"
	if err := insert(key, owner, key+".zip", password, path, "{}", scope); err != nil {
		t.Fatalf("insert %s: %v", key, err)
	}
	if storeBody {
		if _, err := objectStorage.Put(context.Background(), path, strings.NewReader("zip-bytes"), 9, "application/zip", nil); err != nil {
			t.Fatalf("put object %s: %v", path, err)
		}
	}
}

func TestDownloadRouteGates(t *testing.T) {
	srv := setupDownloadRoute(t)

	seedObject(t, "pub", "alice", "", ScopePublic, true)
	seedObject(t, "pwd", "alice", "s3cret", ScopePublic, true)
	seedObject(t, "gated", "alice", "", ScopeAuthenticated, true)
	seedObject(t, "broken", "alice", "", ScopePublic, false) // index entry, no backend object

	cases := []struct {
		name       string
		url        string
		authz      string
		wantStatus int
		wantError  string // "" means expect zip stream
		wantGate   string
	}{
		{"1 key not found", "/download/nope.zip", "", 404, "not found", ""},
		{"2 public file", "/download/pub.zip", "", 200, "", ""},
		{"3 password correct", "/download/pwd.zip?password=s3cret", "", 200, "", ""},
		{"4 password missing", "/download/pwd.zip", "", 401, "password required", "password"},
		{"5 password wrong", "/download/pwd.zip?password=nope", "", 401, "password required", "password"},
		{"6 auth-gated session ignored", "/download/gated.zip", "Bearer valid-alice", 401, "login required", "auth"},
		{"7 auth-gated no header", "/download/gated.zip", "", 401, "login required", "auth"},
		{"8 backend read fail", "/download/broken.zip", "", 500, "internal error", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", srv.URL+tc.url, nil)
			if tc.authz != "" {
				req.Header.Set("Authorization", tc.authz)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status: got %d want %d", resp.StatusCode, tc.wantStatus)
			}
			if tc.wantError == "" {
				return // 200: zip stream, status check suffices
			}
			var body map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["error"] != tc.wantError {
				t.Fatalf("error: got %q want %q", body["error"], tc.wantError)
			}
			if body["gate"] != tc.wantGate {
				t.Fatalf("gate: got %q want %q", body["gate"], tc.wantGate)
			}
		})
	}
}

func TestDownloadRouteOwnerScope(t *testing.T) {
	srv := setupDownloadRoute(t)
	seedObject(t, "mine", "alice", "", ScopeOwner, true)

	// This route ignores auth entirely: even the owner gets the auth gate.
	req, _ := http.NewRequest("GET", srv.URL+"/download/mine.zip", nil)
	req.Header.Set("Authorization", "Bearer valid-alice")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("owner-scope download: got %d want 401", resp.StatusCode)
	}
}
