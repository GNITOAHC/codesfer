package auth

import "net/http"

func UsernameFromSessionID(sessionID string) (string, error) {
	session, err := getSession(sessionID)
	if err != nil {
		return "", err
	}
	user, err := getUser(session.Email)
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

func UpdateSessionLastSeen(sessionID string) error {
	return updateSessionLastSeen(sessionID)
}

// RequireUser rejects unauthenticated requests and passes the caller's username
// (set by the auth middleware) to the handler. action names the operation in
// the error message.
func RequireUser(action string, handler func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.Header.Get("X-Username")
		if username == "" {
			http.Error(w, "unauthorized, only authorized users can "+action, http.StatusUnauthorized)
			return
		}
		handler(w, r, username)
	}
}
