package auth

import (
	"sync"
	"time"

	"github.com/gnitoahc/codesfer/internal/constants"
)

// AnonUsername owns every anonymous upload and short url. It is a reserved
// username, so no account can claim it.
const AnonUsername = "anon"

// anonWindow counts one IP's anonymous requests in the current window.
type anonWindow struct {
	start time.Time
	count int
}

// ponytail: in-memory fixed window, so limits are per process and reset on
// restart. Move the counters to the database or Redis if the server is ever
// run as more than one instance.
var (
	anonWindows   = map[string]*anonWindow{}
	anonWindowsMu sync.Mutex
)

// AllowAnon reports whether ip may upload anonymously now, counting the
// attempt when it may.
func AllowAnon(ip string, now time.Time) bool {
	anonWindowsMu.Lock()
	defer anonWindowsMu.Unlock()

	win, ok := anonWindows[ip]
	if !ok || now.Sub(win.start) >= constants.AnonUploadWindow {
		anonWindows[ip] = &anonWindow{start: now, count: 1}
		return true
	}
	if win.count >= constants.AnonUploadLimit {
		return false
	}
	win.count++
	return true
}

// PruneAnonWindows drops rate-limit windows that have already ended, so the
// map only holds IPs seen recently.
func PruneAnonWindows(now time.Time) {
	anonWindowsMu.Lock()
	defer anonWindowsMu.Unlock()
	for ip, win := range anonWindows {
		if now.Sub(win.start) >= constants.AnonUploadWindow {
			delete(anonWindows, ip)
		}
	}
}
