package pull

import (
	"codesfer/internal/backend"
	"log"
)

type Flags struct {
	Out  string
	Pass string
}

func Run(flags Flags, code string) {
	sessionID := backend.ReadSessionID()
	if sessionID == "" {
		log.Printf("Not logged in")
	}

	zip, err := backend.Pull(sessionID, code, flags.Pass)
	if err != nil {
		log.Fatalf("Pull failed: %v", err)
	}

	log.Printf("File downloaded: %s", zip)
	log.Printf("Decompressing to %s", flags.Out)

	err = backend.Decompress(zip, flags.Out)
	if err != nil {
		log.Fatalf("Decompress failed: %v", err)
	}
}
