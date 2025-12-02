package cli

import (
	"codesfer/internal/client"
	"log"
)

type PullFlags struct {
	Out  string
	Pass string
}

func Pull(flags PullFlags, code string) {
	sessionID := client.ReadSessionID()
	if sessionID == "" {
		log.Printf("Not logged in")
	}

	log.Print("Pulling...")
	zip, err := client.Pull(sessionID, code, flags.Pass)
	if err != nil {
		log.Fatalf("Pull failed: %v", err)
	}

	log.Printf("File downloaded: %s", zip)
	log.Printf("Decompressing to %s", flags.Out)

	err = client.Decompress(zip, flags.Out)
	if err != nil {
		log.Fatalf("Decompress failed: %v", err)
	}
}
