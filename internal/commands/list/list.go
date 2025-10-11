package list

import (
	"codesfer/internal/backend"
	"fmt"
	"log"
)

func Run() {
	sessionID := backend.ReadSessionID()
	if sessionID == "" {
		log.Fatal("You are not logged in. Login first to list codes.")
	}

	objs, err := backend.List(sessionID)
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
		fmt.Printf("[%s] %s (pass: %s; created at: %s)\n", obj.ID, obj.Filename, pass, obj.CreatedAt)
	}
}
