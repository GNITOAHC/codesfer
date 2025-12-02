// Package server provides functionalities to start and manage the server.
package server

import (
	"codesfer/internal/server/auth"
	"codesfer/internal/server/storage"
	"codesfer/pkg/r2"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/gnitoahc/go-dotenv"
)

var (
	port = flag.Int("port", 3000, "The server port")
)

func init() {
	dotenv.Load(".env")
}

func Serve() {
	driver := dotenv.Get("DB_DRIVER", "sqlite")
	source := dotenv.Get("DB_SOURCE", "file:auth.db?cache=shared")
	indexDriver := dotenv.Get("INDEX_DB_DRIVER", "sqlite")
	indexSource := dotenv.Get("INDEX_DB_SOURCE", "file:index.db?cache=shared")

	r2Storage := r2.Storage{}
	if err := r2Storage.Init(context.Background(), r2.Config{
		AccountID:       os.Getenv("CF_ACCOUNT_ID"),
		AccessKey:       os.Getenv("CF_ACCESS_KEY"),
		SecretAccessKey: os.Getenv("CF_SECRET_ACCESS_KEY"),
		Bucket:          os.Getenv("CF_BUCKET"),
	}); err != nil {
		panic(err)
	}

	// Mux definition start
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})
	handle(mux, "/auth/", http.StripPrefix("/auth", auth.AuthHandler(driver, source)))
	handle(mux, "/storage/", http.StripPrefix("/storage", storage.StorageHandler(indexDriver, indexSource, &r2Storage)), authMiddleware)
	// Mux definition end

	log.Printf("Starting server on port %d", *port)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Fatal(http.Serve(lis, mux))
}
