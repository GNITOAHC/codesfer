package storage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gnitoahc/codesfer/pkg/sqlite"
)

func setupRemoveRoute(t *testing.T) *httptest.Server {
	t.Helper()
	setupIndexDB(t)

	backend := &sqlite.Storage{}
	source := fmt.Sprintf("file:%s?cache=shared&mode=rwc", filepath.Join(t.TempDir(), "objects.db"))
	if err := backend.Init(context.Background(), sqlite.Config{Source: source, Table: "objects"}); err != nil {
		t.Fatalf("init object storage: %v", err)
	}
	objectStorage = backend

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /remove", func(w http.ResponseWriter, r *http.Request) {
		remove(w, r, r.Header.Get("X-Username"), r.URL.Query().Get("key"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRemoveStatusCodes(t *testing.T) {
	srv := setupRemoveRoute(t)
	seedObject(t, "mine", "alice", "", ScopePublic, true)
	seedObject(t, "theirs", "bob", "", ScopePublic, true)

	cases := []struct {
		name string
		key  string
		want int
	}{
		{"removed", "mine", http.StatusNoContent},
		{"already removed", "mine", http.StatusNotFound},
		{"owned by someone else", "theirs", http.StatusNotFound},
		{"missing key", "", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("DELETE", srv.URL+"/remove?key="+tc.key, nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("X-Username", "alice")

			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.want {
				t.Fatalf("status: got %d want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
