// Package reply writes the JSON responses shared by the server's routes.
package reply

import (
	"encoding/json"
	"net/http"
)

// JSON writes v as a 200 JSON response.
func JSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// Error writes {"error": msg} with status. A non-empty gate ("auth" or
// "password") tells the frontend which prompt to show.
func Error(w http.ResponseWriter, status int, msg, gate string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]string{"error": msg}
	if gate != "" {
		resp["gate"] = gate
	}
	json.NewEncoder(w).Encode(resp)
}
