package url

import (
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/gnitoahc/codesfer/internal/server/auth"
)

var unlockPage = template.Must(template.New("unlock").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Protected link</title>
<style>body{font:16px system-ui,sans-serif;max-width:24rem;margin:4rem auto;padding:0 1rem}input,button{font:inherit;padding:.4rem}.err{color:#c00}</style>
</head><body>
<form method="post">
<p>This link is password protected.</p>
{{if .}}<p class="err">{{.}}</p>{{end}}
<label>Password <input type="password" name="password" autofocus required></label>
<button>Open</button>
</form>
</body></html>`))

// RedirectRoute serves /r/{handle} and /u/{username}/{custom}. A protected
// link answers GET with a password form that POSTs back here, so the password
// never lands in the url.
//
//	curl -i http://base_url/r/<handle>
func RedirectRoute(w http.ResponseWriter, r *http.Request) {
	var (
		l   *link
		err error
	)
	if handle := r.PathValue("handle"); handle != "" {
		l, err = get(handle)
	} else {
		l, err = getByCustom(r.PathValue("username"), r.PathValue("custom"))
	}
	if err != nil {
		log.Printf("[redirect] lookup error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if l == nil {
		http.NotFound(w, r)
		return
	}

	if l.Password != "" {
		if r.Method != http.MethodPost {
			unlock(w, http.StatusUnauthorized, "")
			return
		}
		// Every attempt counts against the anonymous per-IP budget, which
		// caps password guessing.
		if !auth.AllowAnon(auth.ClientIP(r), time.Now()) {
			unlock(w, http.StatusTooManyRequests, "Too many attempts, try again later.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxForm)
		if !passwordOK(l, r.PostFormValue("password")) {
			unlock(w, http.StatusUnauthorized, "Wrong password.")
			return
		}
	}

	http.Redirect(w, r, l.Target, http.StatusFound)
}

func unlock(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	unlockPage.Execute(w, msg)
}
