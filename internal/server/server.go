// Package server provides functionalities to start and manage the server.
package server

import (
	"context"
	"fmt"
	"github.com/gnitoahc/codesfer/internal/server/auth"
	"github.com/gnitoahc/codesfer/internal/server/storage"
	"github.com/gnitoahc/codesfer/pkg/cron"
	"github.com/gnitoahc/codesfer/pkg/object"
	"github.com/gnitoahc/codesfer/pkg/r2"
	"github.com/gnitoahc/codesfer/pkg/sqlite"
	"log"
	"net"
	"net/http"

	"github.com/gnitoahc/go-dotenv"
)

type ServeFlags struct {
	Port   int
	Dotenv string
}

func Serve(flags ServeFlags) {
	dotenv.Load(flags.Dotenv) // Default to .env in cmd/codesfer-server/main.go
	log.Println("Environment variables loaded from", flags.Dotenv)

	driver := dotenv.Get("DB_DRIVER", "sqlite")
	source := dotenv.Get("DB_SOURCE", "file:auth.db?cache=shared")
	indexDriver := dotenv.Get("INDEX_DB_DRIVER", "sqlite")
	indexSource := dotenv.Get("INDEX_DB_SOURCE", "file:index.db?cache=shared")
	backendDriver := dotenv.Get("OBJECT_STORAGE_DRIVER", "sqlite")

	var backend object.ObjectStorage
	switch backendDriver {
	case "r2":
		log.Println("Using R2 as object storage backend")
		backend = &r2.Storage{}
		if err := backend.Init(context.Background(), r2.Config{
			AccountID:       dotenv.Must("CF_ACCOUNT_ID"),
			AccessKey:       dotenv.Must("CF_ACCESS_KEY"),
			SecretAccessKey: dotenv.Must("CF_SECRET_ACCESS_KEY"),
			Bucket:          dotenv.Must("CF_BUCKET"),
		}); err != nil {
			panic(err)
		}
	case "sqlite":
		log.Println("Using SQLite as object storage backend")
		backend = &sqlite.Storage{}
		if err := backend.Init(context.Background(), sqlite.Config{
			Source: dotenv.Get("OBJECT_STORAGE_SOURCE", "file:object_storage.db?cache=shared"),
		}); err != nil {
			panic(err)
		}
	default:
		panic(fmt.Sprintf("unknown backend driver: %s", backendDriver))
	}

	// Mux definition start
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})
	handle(mux, "/auth/", http.StripPrefix("/auth", auth.AuthHandler(driver, source)))
	handle(mux, "/storage/", http.StripPrefix("/storage", storage.StorageHandler(indexDriver, indexSource, backend)), authMiddleware)
	handle(mux, "GET /download/{key}", http.HandlerFunc(storage.DownloadRoute))
	// Mux definition end

	log.Printf("Starting server on port %d", flags.Port)

	// Start cron jobs
	cronMgr := cron.NewManager()
	AddCronJobs(cronMgr)
	cronMgr.Start()
	defer cronMgr.Stop()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", flags.Port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Fatal(http.Serve(lis, mux))
}
