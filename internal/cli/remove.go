package cli

import (
	"codesfer/internal/client"
	"log"
)

func Remove(codes []string) {
	const (
		colorYellow = "\033[33m"
		colorReset  = "\033[0m"
	)

	sessionID := client.ReadSessionID()
	if sessionID == "" {
		log.Fatal("You are not logged in. Login first.")
	}

	log.Printf("Removing %d code(s)...", len(codes))

	resp, err := client.Remove(sessionID, codes)
	if err != nil {
		log.Fatal(err)
	}

	for code, status := range resp.Results {
		log.Printf("[%s] %s%s%s", code, colorYellow, status, colorReset)
	}
}
