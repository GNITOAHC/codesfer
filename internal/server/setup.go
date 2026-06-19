package server

import (
	"fmt"
	"log"
	"os"
)

func SetupEnv(dotenvPath string) {
	initEnv := "" +
		"##################\n" + //
		"# Codeserver Env #\n" + //
		"##################\n" +
		"\n" +
		"AUTH_DB_DRIVER=sqlite\n" +
		"AUTH_DB_SOURCE=file:auth.db?cache=shared\n" +
		"INDEX_DB_DRIVER=sqlite\n" +
		"INDEX_DB_SOURCE=file:object_storage.db?cache=shared\n" +
		"OBJECT_STORAGE_DRIVER=sqlite # or r2\n" +
		"OBJECT_STORAGE_SOURCE=file:object_storage.db?cache=shared\n" +
		"\n" +
		"# If using R2 as object storage backend, set the following:\n" +
		"CF_ACCOUNT_ID=\n" +
		"CF_ACCESS_KEY=\n" +
		"CF_SECRET_ACCESS_KEY=\n" +
		"CF_BUCKET=\n" +
		"\n" +
		"# For more info: https://codesfer.io/self-hosting\n"

	if dotenvPath != "" {
		if _, err := os.Stat(dotenvPath); err == nil {
			log.Fatalf("%s already exists.", dotenvPath)
		} else if !os.IsNotExist(err) {
			log.Fatal(err)
		}

		err := os.WriteFile(dotenvPath, []byte(initEnv), 0644)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Created " + dotenvPath + " file with default values.")
		return
	}

	// Check if .env file already exists, if not, create one with default values
	if _, err := os.Stat(".env"); os.IsNotExist(err) {
		err := os.WriteFile(".env", []byte(initEnv), 0644)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Created .env file with default values.")
		return
	}

	// If .env file exists, check .env.1, .env.2, ... until we find a free name
	i := 1
	for {
		if _, err := os.Stat(".env." + fmt.Sprint(i)); os.IsNotExist(err) {
			err := os.WriteFile(".env."+fmt.Sprint(i), []byte(initEnv), 0644)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Created .env." + fmt.Sprint(i) + " file with default values.")
			return
		}
		i++
	}
}
