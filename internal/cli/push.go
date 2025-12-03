package cli

import (
	"codesfer/internal/client"
	"crypto/rand"
	"fmt"
	"log"
	"os"
)

type PushFlags struct {
	Path string
	Pass string
	Key  string
	Desc string
}

func randID(n int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range n {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}

func Push(flags PushFlags, args []string) {
	const (
		colorYellow = "\033[33m"
		colorReset  = "\033[0m"
	)

	path := flags.Path

	sessionID := client.ReadSessionID()
	if path == "" {
		log.Print("Pushing code...")
	} else {
		if sessionID == "" {
			log.Fatal("You are not logged in. Login first to specify code name or push without name.")
		}
		log.Printf("Pushing code with name: %s%s%s", colorYellow, path, colorReset)
	}

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

	if path == "" {
		path, err = randID(5)
		if err != nil {
			log.Fatal(err)
		}
	}

	log.Printf("Uploading ...")
	form := client.PushForm{
		Key:      flags.Key,
		Path:     path,
		Password: flags.Pass,
	}
	uid, err := client.Push(form, f.Name())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(uid)
}
