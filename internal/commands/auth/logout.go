package auth

import (
	"codesfer/internal/backend"
	"fmt"
	"log"
)

func Logout() {
	sessionID := backend.ReadSessionID()
	err := backend.Logout(sessionID)
	if err != nil {
		log.Println("Session not deleted or not found:", err)
	}
	err = backend.RemoveSessionID()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Logout successful.")
}
