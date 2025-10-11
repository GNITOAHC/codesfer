package push

import (
	"codesfer/internal/backend"
	"crypto/rand"
	"fmt"
	"log"
	"os"
)

type Flags struct {
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

func Run(flags Flags, args []string) {
	const (
		colorRed    = "\033[31m"
		colorGreen  = "\033[32m"
		colorYellow = "\033[33m"
		colorReset  = "\033[0m"
	)

	path := flags.Path

	sessionID := backend.ReadSessionID()
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
	backend.CompressFiles(args, f.Name())

	// log.Printf("dir: %s", dir)
	// log.Printf("file: %s", file)
	// log.Printf("f.Name(): %s", f.Name())
	// log.Printf("f.Name() ext: %s", strings.Split(f.Name(), ".")[len(strings.Split(f.Name(), "."))-1])

	// log.Printf("Pushing to path %s%s%s", colorYellow, path, colorReset)

	if path == "" {
		path, err = randID(5)
		if err != nil {
			log.Fatal(err)
		}
	}

	form := backend.PushForm{
		Key:      flags.Key,
		Path:     path,
		Password: flags.Pass,
	}
	uid, err := backend.Push(form, f.Name(), sessionID == "")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(uid)
}
