package cli

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/gnitoahc/codesfer/internal/client"
	"github.com/gnitoahc/codesfer/internal/constants"
	"github.com/gnitoahc/codesfer/pkg/api"
)

type PushFlags struct {
	Path   string
	Pass   string
	Key    string
	Desc   string
	Force  bool
	Access string
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

	// Validate before compressing anything; the server rejects unknown scopes too.
	switch flags.Access {
	case "", "owner", "authenticated", "public":
	default:
		log.Fatalf("invalid --access %q (want owner, authenticated or public)", flags.Access)
	}

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

	// Collect file tree for metadata
	tree, err := client.CollectFileTree(args)
	if err != nil {
		log.Fatalf("Failed to collect file tree: %v", err)
	}

	// Build metadata
	metadata := map[string]any{
		"tree": tree,
	}
	if flags.Desc != "" {
		metadata["desc"] = flags.Desc
	}

	zipInfo, err := os.Stat(f.Name())
	if err != nil {
		log.Fatalf("Failed to stat zip file: %v", err)
	}
	log.Printf("Compressed size: %s", formatSize(zipInfo.Size()))

	form := client.PushForm{
		Key:      flags.Key,
		Path:     customPath,
		Password: flags.Pass,
		Force:    flags.Force,
		Access:   flags.Access,
		Metadata: metadata,
	}

	var resp *api.UploadResponse
	if constants.UploadChunkSize < zipInfo.Size() {
		log.Printf("File exceeds 90 MB, switching to chunked upload")
		resp, err = client.PushChunked(form, f.Name())
	} else {
		log.Printf("Uploading ...")
		resp, err = client.Push(form, f.Name())
	}
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("ID: %s\n", resp.Uid)
	fmt.Printf("Path: %s\n", resp.Path)
}

func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
