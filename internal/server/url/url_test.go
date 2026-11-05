package url

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gnitoahc/codesfer/internal/constants"
	"github.com/gnitoahc/codesfer/internal/testdb"
	"github.com/gnitoahc/codesfer/pkg/api"
)

const testBase = "https://lnk.test"

// ipSeq hands every request helper its own client IP, so the shared anonymous
// rate limiter never carries over between tests.
var ipSeq atomic.Int64

func freshIP() string {
	n := ipSeq.Add(1)
	return fmt.Sprintf("10.%d.%d.%d", n>>16&255, n>>8&255, n&255)
}

// setup wires the /url routes and the redirect routes the way server.go does,
// minus authMiddleware: tests set X-Username themselves; empty = anonymous.
func setup(t *testing.T) *httptest.Server {
	t.Helper()
	driver, source := testdb.Open(t)
	h := Handler(driver, source, testBase)
	t.Cleanup(func() { db.Close() })

	mux := http.NewServeMux()
	mux.Handle("/url/", http.StripPrefix("/url", h))
	for _, p := range []string{"GET /r/{handle}", "POST /r/{handle}", "GET /u/{username}/{custom}", "POST /u/{username}/{custom}"} {
		mux.HandleFunc(p, RedirectRoute)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// noFollow keeps redirects visible to the test.
var noFollow = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func do(t *testing.T, method, rawURL, username, ip string, form neturl.Values) *http.Response {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("X-Username", username)
	req.Header.Set("CF-Connecting-IP", ip)
	resp, err := noFollow.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func newURL(t *testing.T, srv *httptest.Server, username string, form neturl.Values) (*http.Response, api.ShortURLResponse) {
	t.Helper()
	resp := do(t, "POST", srv.URL+"/url/new", username, freshIP(), form)
	var out api.ShortURLResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return resp, out
}

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: got %d want %d: %s", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, want, b)
	}
}

func TestAnonymousLink(t *testing.T) {
	srv := setup(t)

	resp, out := newURL(t, srv, "", neturl.Values{"target": {"https://example.com/a"}})
	wantStatus(t, resp, http.StatusOK)
	if out.ExpiresAt == 0 || out.CustomURL != "" || out.URL != testBase+"/r/"+out.Handle {
		t.Fatalf("unexpected response: %+v", out)
	}

	resp = do(t, "GET", srv.URL+"/r/"+out.Handle, "", freshIP(), nil)
	wantStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://example.com/a" {
		t.Fatalf("Location: got %q", loc)
	}

	resp, _ = newURL(t, srv, "", neturl.Values{"target": {"https://example.com"}, "custom": {"mine"}})
	wantStatus(t, resp, http.StatusUnauthorized)
}

func TestAnonymousRateLimit(t *testing.T) {
	srv := setup(t)
	ip := freshIP()
	form := neturl.Values{"target": {"https://example.com"}}
	for i := 0; i < constants.AnonUploadLimit; i++ {
		wantStatus(t, do(t, "POST", srv.URL+"/url/new", "", ip, form), http.StatusOK)
	}
	wantStatus(t, do(t, "POST", srv.URL+"/url/new", "", ip, form), http.StatusTooManyRequests)
}

func TestCustomLink(t *testing.T) {
	srv := setup(t)

	resp, out := newURL(t, srv, "alice", neturl.Values{"target": {"https://example.com/c"}, "custom": {"demo_1"}})
	wantStatus(t, resp, http.StatusOK)
	if out.ExpiresAt != 0 || out.CustomURL != testBase+"/u/alice/demo_1" {
		t.Fatalf("unexpected response: %+v", out)
	}
	wantStatus(t, do(t, "GET", srv.URL+"/u/alice/demo_1", "", freshIP(), nil), http.StatusFound)
	wantStatus(t, do(t, "GET", srv.URL+"/r/"+out.Handle, "", freshIP(), nil), http.StatusFound)

	resp, _ = newURL(t, srv, "alice", neturl.Values{"target": {"https://example.com"}, "custom": {"demo_1"}})
	wantStatus(t, resp, http.StatusConflict)
	// Same custom name under another user is fine.
	resp, _ = newURL(t, srv, "bob", neturl.Values{"target": {"https://example.com"}, "custom": {"demo_1"}})
	wantStatus(t, resp, http.StatusOK)

	for _, bad := range []string{"1ab", "ab", "a/b", "a-b", strings.Repeat("a", 33)} {
		resp, _ = newURL(t, srv, "alice", neturl.Values{"target": {"https://example.com"}, "custom": {bad}})
		wantStatus(t, resp, http.StatusBadRequest)
	}
}

func TestBadTarget(t *testing.T) {
	srv := setup(t)
	for _, bad := range []string{"", "javascript:alert(1)", "ftp://x.com", "example.com", testBase + "/r/abc", "https://" + strings.Repeat("a", maxTarget)} {
		resp, _ := newURL(t, srv, "alice", neturl.Values{"target": {bad}})
		wantStatus(t, resp, http.StatusBadRequest)
	}
}

func TestPasswordUnlock(t *testing.T) {
	srv := setup(t)
	resp, out := newURL(t, srv, "alice", neturl.Values{"target": {"https://example.com/p"}, "password": {"s3cret"}})
	wantStatus(t, resp, http.StatusOK)
	link := srv.URL + "/r/" + out.Handle

	resp = do(t, "GET", link, "", freshIP(), nil)
	wantStatus(t, resp, http.StatusUnauthorized)
	if b, _ := io.ReadAll(resp.Body); !strings.Contains(string(b), `<form method="post">`) {
		t.Fatalf("GET protected link: want unlock form, got %s", b)
	}
	wantStatus(t, do(t, "POST", link, "", freshIP(), neturl.Values{"password": {"nope"}}), http.StatusUnauthorized)
	resp = do(t, "POST", link, "", freshIP(), neturl.Values{"password": {"s3cret"}})
	wantStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://example.com/p" {
		t.Fatalf("Location: got %q", loc)
	}

	// info: owner bypasses the password and sees it; others must supply it.
	resp = do(t, "GET", srv.URL+"/url/info?key="+out.Handle, "alice", freshIP(), nil)
	wantStatus(t, resp, http.StatusOK)
	var got api.ShortURLResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Password != "s3cret" || !got.Protected {
		t.Fatalf("owner info: %+v", got)
	}
	wantStatus(t, do(t, "GET", srv.URL+"/url/info?key="+out.Handle, "bob", freshIP(), nil), http.StatusUnauthorized)
	resp = do(t, "GET", srv.URL+"/url/info?key="+out.Handle+"&password=s3cret", "bob", freshIP(), nil)
	wantStatus(t, resp, http.StatusOK)
	got = api.ShortURLResponse{}
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Password != "" {
		t.Fatalf("non-owner info leaked password: %+v", got)
	}
}

func TestListAndRemove(t *testing.T) {
	srv := setup(t)
	_, a := newURL(t, srv, "alice", neturl.Values{"target": {"https://example.com/1"}})
	_, b := newURL(t, srv, "alice", neturl.Values{"target": {"https://example.com/2"}, "custom": {"two"}})
	newURL(t, srv, "bob", neturl.Values{"target": {"https://example.com/3"}})

	resp := do(t, "GET", srv.URL+"/url/list", "alice", freshIP(), nil)
	wantStatus(t, resp, http.StatusOK)
	var links api.ShortURLListResponse
	json.NewDecoder(resp.Body).Decode(&links)
	if len(links) != 2 {
		t.Fatalf("alice list: got %d links want 2", len(links))
	}

	wantStatus(t, do(t, "DELETE", srv.URL+"/url/remove?key="+a.Handle, "bob", freshIP(), nil), http.StatusNotFound)
	wantStatus(t, do(t, "DELETE", srv.URL+"/url/remove?key="+a.Handle, "alice", freshIP(), nil), http.StatusNoContent)
	wantStatus(t, do(t, "DELETE", srv.URL+"/url/remove?key=alice/two", "alice", freshIP(), nil), http.StatusNoContent)
	wantStatus(t, do(t, "GET", srv.URL+"/r/"+a.Handle, "", freshIP(), nil), http.StatusNotFound)
	wantStatus(t, do(t, "GET", srv.URL+"/r/"+b.Handle, "", freshIP(), nil), http.StatusNotFound)
	wantStatus(t, do(t, "GET", srv.URL+"/u/alice/two", "", freshIP(), nil), http.StatusNotFound)
}

func TestExpiredLink(t *testing.T) {
	srv := setup(t)
	past := time.Now().Add(-time.Minute).Unix()
	if _, err := insert(&link{Handle: "old", Username: "anon", Target: "https://example.com", CreatedAt: past, ExpiresAt: past}); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, do(t, "GET", srv.URL+"/r/old", "", freshIP(), nil), http.StatusNotFound)

	if err := SweepExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow("SELECT COUNT(*) FROM " + tableName).Scan(&n)
	if n != 0 {
		t.Fatalf("after sweep: %d rows left", n)
	}
}
