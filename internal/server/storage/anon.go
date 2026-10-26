package storage

import (
	"sync"
	"time"

	"github.com/gnitoahc/codesfer/internal/constants"
)

// anonUsername owns every anonymous upload. It is a reserved username, so no
// account can claim it.
const anonUsername = "anon"

// anonFormSlack is headroom over the file size cap for the multipart framing
// and text fields around it.
const anonFormSlack = 1 << 20

// anonWindow counts one IP's anonymous uploads in the current window.
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

// allowAnonUpload reports whether ip may upload anonymously now, counting the
// attempt when it may.
func allowAnonUpload(ip string, now time.Time) bool {
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
