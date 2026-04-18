package cli

import (
	"fmt"
	"github.com/gnitoahc/codesfer/internal/client"
	"log"
	"sort"
	"time"
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

	sort.Slice(objs, func(i, j int) bool {
		return objs[i].CreatedAt > objs[j].CreatedAt
	})

	const (
		colorReset  = "\033[0m"
		colorYellow = "\033[33m" // key
		colorWhite  = "\033[97m" // path
		colorCyan   = "\033[36m" // password
		colorGray   = "\033[90m" // created at
	)

	for _, obj := range objs {
		var pass string
		if obj.Password == "" {
			pass = colorGray + "<none>" + colorReset
		} else {
			pass = colorCyan + obj.Password + colorReset
		}
		fmt.Printf("[%s%s%s] %s%s%s (pass: %s; created at: %s%s%s)\n",
			colorYellow, obj.Key, colorReset,
			colorWhite, obj.Path, colorReset,
			pass,
			colorGray, time.Unix(obj.CreatedAt, 0).Format("2006-01-02 15:04:05"), colorReset,
		)
	}

	if len(objs) == 0 {
		fmt.Println("No codes found.")
	}

	fmt.Printf("\nShare it even more easily with link: %s%s/download/<code>.zip%s\n", colorCyan, client.BaseURL, colorReset)
}
