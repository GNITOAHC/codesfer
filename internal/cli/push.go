package cli

import (
	"codesfer/internal/client"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
)

type PushFlags struct {
	Path string
	Pass string
	Key  string
	Desc string
}

// sanitizePath ensures the path contains only allowed characters i.e. A~Z, a~z, 0~9, _, - and /
func sanitizePath(p string) string {
	var b strings.Builder
	for _, r := range p {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '_' || r == '-' || r == '/' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func getPath(flags PushFlags, args []string) string {
	// if flag path is not set, get from args
	// if only one file is provided in args, use that as path
	// otherwise, use current directory's name
	if p := sanitizePath(flags.Path); p != "" {
		return p
	}

	if len(args) == 1 {
		if p := sanitizePath(path.Base(args[0])); p != "" {
			return p
		}
	}

	// prevent user inputing first argument contains only ".", "./" or similar
	wd := os.Getenv("PWD")
	if wd == "" {
		if cwd, err := os.Getwd(); err == nil {
			wd = cwd
		}
	}

	// Make sure the path contains only A~Z, a~z, 0~9, _, - and /
	return sanitizePath(path.Base(wd))
}

func Push(flags PushFlags, args []string) {
	const (
		colorYellow = "\033[33m"
		colorReset  = "\033[0m"
	)

	customPath := getPath(flags, args)

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
