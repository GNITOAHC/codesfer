package cli

import (
	"codesfer/internal/client"
	"fmt"
	"log"
)

// List displays all code snippets for the logged-in user.
func List() {
	sessionID := client.ReadSessionID()
	if sessionID == "" {
		log.Fatal("You are not logged in. Login first to list codes.")
	}

	objs, err := client.List(sessionID)
	if err != nil {
		log.Fatal(err)
	}

	for _, obj := range objs {
		var pass string
		if obj.Password == "" {
			pass = "<none>"
		} else {
			pass = obj.Password
		}
		fmt.Printf("[%s] %s (pass: %s; created at: %s)\n", obj.Key, obj.Path, pass, obj.CreatedAt)
	}
}
