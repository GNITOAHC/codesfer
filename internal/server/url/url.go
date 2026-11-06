// Package url provides the short url routes.
package url

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gnitoahc/codesfer/internal/constants"
	"github.com/gnitoahc/codesfer/internal/server/auth"
	"github.com/gnitoahc/codesfer/internal/server/reply"
	"github.com/gnitoahc/codesfer/pkg/api"
	"github.com/gnitoahc/codesfer/pkg/randid"
)

const (
	handleLen = 8
	maxForm   = 64 << 10
	maxTarget = 2048
)

var customPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{2,31}$`)

// linkBase is the public base of every short url, e.g. https://lnk.codesfer.io.
// Full urls are built from it per response and never stored, so changing it
// moves every existing link.
var (
	linkBase string
	linkHost string
)

func Handler(driver, source, base string) http.Handler {
	if err := connect(driver, source); err != nil {
		panic(err)
	}
	linkBase = strings.TrimRight(base, "/")
	u, err := neturl.Parse(linkBase)
	if err != nil || u.Host == "" {
		panic("invalid LINK_BASE_URL: " + base)
	}
	linkHost = u.Hostname()

	urlHandler := http.NewServeMux()
	urlHandler.HandleFunc("POST /new", create) // anonymous allowed
	urlHandler.HandleFunc("GET /list", auth.RequireUser("list urls", listRoute))
	urlHandler.HandleFunc("DELETE /remove", auth.RequireUser("remove urls", removeRoute))
	urlHandler.HandleFunc("GET /info", info)
	return urlHandler
}

// SweepExpired deletes expired links.
func SweepExpired(ctx context.Context) error {
	n, err := removeExpired()
	if err != nil {
		return err
	}
	if n > 0 {
		log.Printf("[url expiry sweep] removed %d expired link(s)", n)
	}
	return nil
}

func validTarget(raw string) error {
	if len(raw) > maxTarget {
		return errors.New("target url is too long")
	}
	u, err := neturl.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("target must be an http(s) url")
	}
	if strings.EqualFold(u.Hostname(), linkHost) {
		return errors.New("target must not be a short url")
	}
	return nil
}

func response(l *link, owner bool) api.ShortURLResponse {
	resp := api.ShortURLResponse{
		Handle:    l.Handle,
		URL:       linkBase + "/r/" + l.Handle,
		Target:    l.Target,
		Protected: l.Password != "",
		CreatedAt: l.CreatedAt,
		ExpiresAt: l.ExpiresAt,
	}
	if l.Custom != "" {
		resp.CustomURL = linkBase + "/u/" + l.Username + "/" + l.Custom
	}
	if owner {
		resp.Password = l.Password
	}
	return resp
}

// find resolves a key (<handle> || <username>/<custom>) to a live link, or nil.
func find(key string) (*link, error) {
	username, custom, ok := strings.Cut(key, "/")
	if !ok {
		return get(key)
	}
	return getByCustom(username, custom)
}

func create(w http.ResponseWriter, r *http.Request) {
	username := r.Header.Get("X-Username")
	r.Body = http.MaxBytesReader(w, r.Body, maxForm)
	if err := r.ParseMultipartForm(maxForm); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		reply.Error(w, http.StatusBadRequest, "failed to parse form: "+err.Error(), "")
		return
	}

	l := &link{
		Username:  username,
		Custom:    r.FormValue("custom"),
		Target:    r.FormValue("target"),
		Password:  r.FormValue("password"),
		CreatedAt: time.Now().Unix(),
	}
	if err := validTarget(l.Target); err != nil {
		reply.Error(w, http.StatusBadRequest, err.Error(), "")
		return
	}
	if username == "" {
		if l.Custom != "" {
			reply.Error(w, http.StatusUnauthorized, "login required for custom url", "auth")
			return
		}
		if !auth.AllowAnon(auth.ClientIP(r), time.Now()) {
			reply.Error(w, http.StatusTooManyRequests, "too many anonymous requests, try again later or log in", "")
			return
		}
		l.Username = auth.AnonUsername
		l.ExpiresAt = time.Now().Add(constants.AnonURLTTL).Unix()
	} else if l.Custom != "" && !customPattern.MatchString(l.Custom) {
		reply.Error(w, http.StatusBadRequest, "custom url must start with a letter and use 3-32 letters, digits or underscores", "")
		return
	}

	// A handle collision is astronomically rare at 64^8, but retry anyway.
	for range 3 {
		handle, err := randid.New(handleLen)
		if err != nil {
			reply.Error(w, http.StatusInternalServerError, err.Error(), "")
			return
		}
		l.Handle = handle
		taken, err := insert(l)
		if errors.Is(err, ErrCustomTaken) {
			reply.Error(w, http.StatusConflict, err.Error(), "")
			return
		}
		if err != nil {
			log.Printf("[/url/new] insert error: %v", err)
			reply.Error(w, http.StatusInternalServerError, "internal error", "")
			return
		}
		if !taken {
			log.Printf("[/url/new] user %s created %s -> %s", l.Username, l.Handle, l.Target)
			reply.JSON(w, response(l, true))
			return
		}
	}
	reply.Error(w, http.StatusInternalServerError, "could not allocate a handle", "")
}

func listRoute(w http.ResponseWriter, r *http.Request, username string) {
	links, err := list(username)
	if err != nil {
		reply.Error(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	resp := make(api.ShortURLListResponse, 0, len(links))
	for i := range links {
		resp = append(resp, response(&links[i], true))
	}
	reply.JSON(w, resp)
}

func removeRoute(w http.ResponseWriter, r *http.Request, username string) {
	key := r.URL.Query().Get("key")
	l, err := find(key)
	if err != nil {
		reply.Error(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	// Someone else's link reads as missing.
	if l == nil || l.Username != username {
		reply.Error(w, http.StatusNotFound, "url "+key+" not found", "")
		return
	}
	if _, err := remove(username, l.Handle); err != nil {
		reply.Error(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// info returns a link's metadata. The owner bypasses the password.
func info(w http.ResponseWriter, r *http.Request) {
	l, err := find(r.URL.Query().Get("key"))
	if err != nil {
		reply.Error(w, http.StatusInternalServerError, err.Error(), "")
		return
	}
	if l == nil {
		reply.Error(w, http.StatusNotFound, "not found", "")
		return
	}
	owner := r.Header.Get("X-Username") == l.Username
	if !owner && !passwordOK(l, r.URL.Query().Get("password")) {
		reply.Error(w, http.StatusUnauthorized, "password required", "password")
		return
	}
	reply.JSON(w, response(l, owner))
}

func passwordOK(l *link, given string) bool {
	return l.Password == "" || subtle.ConstantTimeCompare([]byte(given), []byte(l.Password)) == 1
}
