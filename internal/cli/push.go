package cli

import (
	"codesfer/internal/client"
	"fmt"
	"log"
	"os"
	"path"
)

type PushFlags struct {
	Path string
	Pass string
	Key  string
	Desc string
}

func Push(flags PushFlags, args []string) {
	const (
		colorYellow = "\033[33m"
		colorReset  = "\033[0m"
	)

	customPath := flags.Path
	if customPath == "" {
		customPath = path.Base(args[0])
	}

	sessionID := client.ReadSessionID()
	if sessionID == "" {
		log.Fatal("You are not logged in. Login first push.")
	}
	log.Printf("Pushing code with name: %s%s%s", colorYellow, customPath, colorReset)

	f, err := os.CreateTemp("", "*.zip")
	if err != nil {
		panic(err)
	}
	defer os.Remove(f.Name()) // ensure cleanup
	for arg := range args {
		log.Printf("Compressing %s", args[arg])
	}
	if err := client.CompressFiles(args, f.Name()); err != nil {
		log.Fatalf("Failed to compress files: %v", err)
	}

	log.Printf("Uploading ...")
	form := client.PushForm{
		Key:      flags.Key,
		Path:     customPath,
		Password: flags.Pass,
	}
	resp, err := client.Push(form, f.Name())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("ID: %s\n", resp.Uid)
	fmt.Printf("Path: %s\n", resp.Path)
}
