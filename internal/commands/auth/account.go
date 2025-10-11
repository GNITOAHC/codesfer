package auth

import (
	"codesfer/internal/backend"
	"fmt"
	"log"
)

// Account shows user's email, username, and all active sessions
func Account() {
	account, err := backend.AccountInfo(backend.ReadSessionID())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Email: %s\n", account.Email)
	fmt.Printf("Username: %s\n", account.Username)
	for i, session := range account.Sessions {
		if session.Current {
			fmt.Printf("[%d] Current session\n", i)
		}
		fmt.Printf(
			"[%d] Session: Location: %s, Agent: %s, Last seen: %s, Created at: %s\n",
			i, session.Location, session.Agent, session.LastSeen, session.CreatedAt,
		)
	}
}

// RevokeSession revokes selected session
func RevokeSession() {
	// TODO
}
