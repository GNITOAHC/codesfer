package auth

import (
	"log"
	"net/http"
)

type middleware func(next http.Handler) http.Handler

func handle(mux *http.ServeMux, pattern string, handler http.Handler, middlewares ...middleware) {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](http.Handler(handler))
	}
	mux.Handle(pattern, handler)
}

// refreshTime will check if user is logged in and also refresh last active timestamp
func refreshTime(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get sessionID
		sessionID := r.Header.Get("Authorization")
		if sessionID == "" {
			sessionID = r.URL.Query().Get("session_id")
		}
		if sessionID == "" {
			http.Error(w, "unauthorized, session not provided, please log in", http.StatusUnauthorized)
			return
		}

		// Remove "Bearer " (only if present, detect to prevent out-of-bounds)
		if len(sessionID) > 7 && sessionID[:7] == "Bearer " {
			sessionID = sessionID[7:]
		}

		// Get username
		username, err := UsernameFromSessionID(sessionID)
		if err != nil {
			http.Error(w, "unauthorized, session not found, please log in", http.StatusUnauthorized)
			return
		}
		// Set custom header
		r.Header.Set("X-Authorized", "true")
		r.Header.Set("X-Session-ID", sessionID)
		r.Header.Set("X-Username", username)

		// Refresh last active timestamp
		err = updateSessionLastSeen(sessionID)
		if err != nil {
			log.Println("[/auth/me] Failed to update session:", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		next.ServeHTTP(w, r)
	})
}
