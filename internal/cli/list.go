package cli

import (
	"fmt"
	"github.com/gnitoahc/codesfer/internal/client"
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

	if len(objs) == 0 {
		fmt.Println("No codes found.")
	}

	fmt.Printf("\nShare it even more easily with link: %s/download/<code>.zip\n", client.BaseURL)
}
