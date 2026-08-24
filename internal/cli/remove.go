package cli

import (
	"log"
	"os"
	"sync"
	"sync/atomic"

	"github.com/gnitoahc/codesfer/internal/client"
)

func Remove(codes []string) {
	const (
		colorRed    = "\033[31m"
		colorYellow = "\033[33m"
		colorReset  = "\033[0m"
	)

	sessionID := client.ReadSessionID()
	if sessionID == "" {
		log.Fatal("You are not logged in. Login first.")
	}

	log.Printf("Removing %d code(s)...", len(codes))

	// one goroutine per key, no worker pool — remove takes a handful of
	// keys from argv. Add a semaphore if that ever stops being true.
	var (
		wg     sync.WaitGroup
		failed atomic.Bool
	)
	for _, code := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := client.Remove(sessionID, code); err != nil {
				failed.Store(true)
				log.Printf("[%s] %s%v%s", code, colorRed, err, colorReset)
				return
			}
			log.Printf("[%s] %sremoved%s", code, colorYellow, colorReset)
		}()
	}
	wg.Wait()

	if failed.Load() {
		os.Exit(1)
	}
}
