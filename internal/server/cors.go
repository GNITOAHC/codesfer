package server

import (
	"net/http"
	"strings"

	"github.com/gnitoahc/go-dotenv"
)

// defaultAllowedOrigins are used when CORS_ALLOWED_ORIGINS is not set.
// codesfer.io is the production web frontend; localhost is for local dev.
var defaultAllowedOrigins = []string{
	"https://codesfer.io", "https://www.codesfer.io",
}

// allowedOrigins loads the CORS allow-list. CORS_ALLOWED_ORIGINS is a
// comma-separated list of origins (e.g. "https://codesfer.io,http://localhost:5173").
func allowedOrigins() map[string]struct{} {
	raw := dotenv.Get("CORS_ALLOWED_ORIGINS", "")
	origins := defaultAllowedOrigins
	if strings.TrimSpace(raw) != "" {
		origins = strings.Split(raw, ",")
	}
	set := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		if o = strings.TrimSpace(o); o != "" {
			set[o] = struct{}{}
		}
	}
	return set
}

// corsMiddleware adds CORS headers so the browser web frontend (codesfer.io)
// can call the API (api.codesfer.io) cross-origin. It reflects the request
// Origin only if it is in the allow-list, and short-circuits OPTIONS preflight
// requests with 204 before they reach auth or business logic.
//
// We reflect a specific origin (never "*") so that credentialed requests are
// supported and the allow-list is enforced. X-Session-ID is exposed because
// login returns the session id in that header and JS must be able to read it.
func corsMiddleware(allowed map[string]struct{}) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := allowed[origin]; ok && origin != "" {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Username, X-Session-ID")
				h.Set("Access-Control-Expose-Headers", "X-Session-ID")
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Max-Age", "86400")
			}
			// Vary on Origin so caches don't serve the wrong CORS headers.
			w.Header().Add("Vary", "Origin")

			// Preflight: answer and stop here, before auth/business logic.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
