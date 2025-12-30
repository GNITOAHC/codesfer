package main

import (
	"codesfer/internal/server"
	"codesfer/pkg/version"
	"fmt"
	"os"
)

func main() {
	// Check for version flag before other processing
	for _, arg := range os.Args[1:] {
		if arg == "-v" || arg == "--version" {
			fmt.Println("codeserver", version.Version)
			os.Exit(0)
		}
	}

	server.Serve()
}
