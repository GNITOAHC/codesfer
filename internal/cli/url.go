package cli

import (
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gnitoahc/codesfer/internal/client"
	"github.com/gnitoahc/codesfer/pkg/api"
)

type URLNewFlags struct {
	Custom string
	Pass   string
}

type URLGetFlags struct {
	Pass string
}

const urlTimeFormat = "2006-01-02 15:04:05"

// URLNew shortens target. Without a login the link is anonymous and expires.
func URLNew(flags URLNewFlags, target string) {
	sessionID := client.ReadSessionID()
	if sessionID == "" && flags.Custom != "" {
		log.Fatal("You are not logged in. Login first to create a custom url.")
	}

	link, err := client.URLNew(sessionID, target, flags.Custom, flags.Pass)
	if err != nil {
		log.Fatalf("Create url failed: %v", err)
	}
	fmt.Println(link.URL)
	if link.CustomURL != "" {
		fmt.Println(link.CustomURL)
	}
	if link.ExpiresAt != 0 {
		fmt.Printf("Expires at %s. Login to create permanent urls.\n", time.Unix(link.ExpiresAt, 0).Format(urlTimeFormat))
	}
}

// URLList displays all short urls of the logged-in user.
func URLList() {
	sessionID := client.ReadSessionID()
	if sessionID == "" {
		log.Fatal("You are not logged in. Login first to list urls.")
	}

	links, err := client.URLList(sessionID)
	if err != nil {
		log.Fatal(err)
	}

	const (
		colorReset  = "\033[0m"
		colorYellow = "\033[33m" // handle
		colorWhite  = "\033[97m" // target
		colorCyan   = "\033[36m" // password, short urls
		colorGray   = "\033[90m" // created at
	)

	for _, link := range links {
		pass := colorGray + "<none>" + colorReset
		if link.Password != "" {
			pass = colorCyan + link.Password + colorReset
		}
		short := link.URL
		if link.CustomURL != "" {
			short += ", " + link.CustomURL
		}
		fmt.Printf("[%s%s%s] %s%s%s -> %s%s%s (pass: %s; created at: %s%s%s)\n",
			colorYellow, link.Handle, colorReset,
			colorCyan, short, colorReset,
			colorWhite, link.Target, colorReset,
			pass,
			colorGray, time.Unix(link.CreatedAt, 0).Format(urlTimeFormat), colorReset,
		)
	}

	if len(links) == 0 {
		fmt.Println("No urls found.")
	}
}

// URLRemove removes short urls by key (<handle> || <username>/<custom>).
func URLRemove(keys []string) {
	const (
		colorRed    = "\033[31m"
		colorYellow = "\033[33m"
		colorReset  = "\033[0m"
	)

	sessionID := client.ReadSessionID()
	if sessionID == "" {
		log.Fatal("You are not logged in. Login first.")
	}

	// Same shape as Remove: one goroutine per key from argv.
	var (
		wg     sync.WaitGroup
		failed atomic.Bool
	)
	for _, key := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := client.URLRemove(sessionID, key); err != nil {
				failed.Store(true)
				log.Printf("[%s] %s%v%s", key, colorRed, err, colorReset)
				return
			}
			log.Printf("[%s] %sremoved%s", key, colorYellow, colorReset)
		}()
	}
	wg.Wait()

	if failed.Load() {
		os.Exit(1)
	}
}

// URLGet prints a short url's target and metadata.
func URLGet(flags URLGetFlags, key string) {
	link, err := client.URLGet(client.ReadSessionID(), key, flags.Pass)
	if err != nil {
		log.Fatalf("Get url failed: %v", err)
	}
	printLink(link)
}

func printLink(link *api.ShortURLResponse) {
	fmt.Printf("Target: %s\n", link.Target)
	fmt.Printf("URL: %s\n", link.URL)
	if link.CustomURL != "" {
		fmt.Printf("Custom URL: %s\n", link.CustomURL)
	}
	fmt.Printf("Protected: %t\n", link.Protected)
	if link.Password != "" {
		fmt.Printf("Password: %s\n", link.Password)
	}
	fmt.Printf("Created: %s\n", time.Unix(link.CreatedAt, 0).Format(urlTimeFormat))
	if link.ExpiresAt != 0 {
		fmt.Printf("Expires: %s\n", time.Unix(link.ExpiresAt, 0).Format(urlTimeFormat))
	}
}
