package server

import (
	"fmt"
	"log"
	"os"
)

func SetupEnv() {
	initEnv := "" +
		"##################\n" + //
		"# Codeserver Env #\n" + //
		"##################\n" +
		"\n" +
		"DB_DRIVER=sqlite\n" +
		"DB_SOURCE=file:auth.db?cache=shared\n" +
		"INDEX_DB=sqlite\n" +
		"INDEX_DB_SOURCE=file:index.db?cache=shared\n" +
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
