package server

import (
	"codesfer/internal/server/auth"
	"net/http"
)

type middleware func(next http.Handler) http.Handler

func handle(mux *http.ServeMux, pattern string, handler http.Handler, middlewares ...middleware) {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](http.Handler(handler))
	}
	mux.Handle(pattern, handler)
}

// authMiddleware will check if user is logged in and assign custom headers to the request
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get sessionID
		sessionID := r.Header.Get("Authorization")
		if sessionID == "" {
			r.Header.Set("X-Authorized", "false")
			r.Header.Set("X-Session-ID", "")
			r.Header.Set("X-Username", "")
			next.ServeHTTP(w, r)
			return
		}

		// Remove "Bearer " (only if present, detect to prevent out-of-bounds)
		if len(sessionID) > 7 && sessionID[:7] == "Bearer " {
			sessionID = sessionID[7:]
		} else {
			r.Header.Set("X-Authorized", "false")
			r.Header.Set("X-Session-ID", "")
			r.Header.Set("X-Username", "")
			next.ServeHTTP(w, r)
			return
		}

		// Get username
		username, err := auth.UsernameFromSessionID(sessionID)
		if err != nil {
			http.Error(w, "unauthorized, session not found, please log in", http.StatusUnauthorized)
			return
		}
		// Set custom header
		r.Header.Set("X-Authorized", "true")
		r.Header.Set("X-Session-ID", sessionID)
		r.Header.Set("X-Username", username)

		auth.UpdateSessionLastSeen(sessionID)

		next.ServeHTTP(w, r)
	})
}
