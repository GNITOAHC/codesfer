package cli

import (
	"github.com/gnitoahc/codesfer/internal/client"
	"log"
)

type PullFlags struct {
	Out  string
	Pass string
	File string
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

	if flags.File != "" {
		log.Printf("Extracting %s to %s", flags.File, flags.Out)
		err = client.DecompressPath(zip, flags.File, flags.Out)
	} else {
		log.Printf("File downloaded: %s", zip)
		log.Printf("Decompressing to %s", flags.Out)
		err = client.Decompress(zip, flags.Out)
	}
	if err != nil {
		log.Fatalf("Decompress failed: %v", err)
	}
}
