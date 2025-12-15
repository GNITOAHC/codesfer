// Package auth provides authentication-related routes.
package auth

import (
	"codesfer/pkg/api"
	"encoding/json"
	"log"
	"net"
	"net/http"
)

var reservedUsername = [3]string{"anon", "admin", "root"}

func AuthHandler(driver, source string) http.Handler {
	// Setup user database
	err := connect(driver, source)
	if err != nil {
		panic(err)
	}

	authhandler := http.NewServeMux()
	authhandler.HandleFunc("GET /username", username)
	authhandler.HandleFunc("POST /register", register)
	authhandler.HandleFunc("POST /login", login)
	authhandler.HandleFunc("POST /logout", logout)
	// authhandler.HandleFunc("GET /me", me)
	handle(authhandler, "/me", http.HandlerFunc(me), refreshTime)

	return authhandler
}

// username route will check if a username is taken
func username(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	for _, reserved := range reservedUsername {
		if username == reserved {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("username forbidden"))
			return
		}
	}
	exists := usernameExists(username)
	if exists {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("username taken"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("username available"))
}

func register(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Username string `json:"username"`
	}
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if data.Email == "" || data.Password == "" || data.Username == "" {
		http.Error(w, "email, password and username are required", http.StatusBadRequest)
		return
	}
	log.Printf("[/auth/register] user %s is trying to register", data.Email)
	err = createUser(data.Email, data.Password, data.Username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[/auth/register] user created: %s", data.Email)
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("user created"))
}

func login(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Username string `json:"username"` // Unimplemented
	}
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		if err.Error() == "user not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("[/auth/login] user %s is trying to login", data.Email)
	verified, err := verify(data.Email, data.Password)
	if err != nil {
		if err.Error() == "user not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
			log.Printf("  [user not found] user %s failed to login", data.Email)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("  [internal error] user %s failed to login", data.Email)
		return
	}
	if !verified {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		log.Printf("  [invalid credentials] user %s failed to login", data.Email)
		return
	}
	agent := r.Header.Get("User-Agent")

	// Get real IP from headers if behind proxy
	ip := r.Header.Get("CF-Connecting-IP") // Cloudflare
	if ip == "" {
		ip = r.Header.Get("X-Forwarded-For") // Standard header
	}
	if ip == "" {
		// Fallback to RemoteAddr
		var err error
		ip, _, err = net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			log.Printf("  failed to split remote addr: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	sessionID, err := createSession(data.Email, agent, ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("X-Session-ID", sessionID)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(sessionID))
}

func logout(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("Authorization")
	if sessionID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sessionID = sessionID[7:] // Remove "Bearer "

	err := deleteSession(sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("logout success"))
}

func me(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := getUserFromSessionID(sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessions, err := getSessions(user.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respSessions := make([]api.AccountSession, 0)

	for _, session := range sessions {
		respSessions = append(respSessions, api.AccountSession{
			Location:  session.Location,
			Agent:     session.Agent,
			LastSeen:  session.LastSeen,
			CreatedAt: session.CreatedAt,
			Current:   session.ID == sessionID,
		})
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(api.AccountResponse{
		Email:    user.Email,
		Username: user.Username,
		Sessions: respSessions,
	})
}
