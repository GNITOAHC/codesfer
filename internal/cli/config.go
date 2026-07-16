package cli

import (
	"fmt"
	"log"

	"github.com/gnitoahc/codesfer/internal/client"
)

// ConfigSet sets a configuration value. The value "default" removes the
// override so the built-in default applies again.
func ConfigSet(key, value string) {
	if value == "default" {
		if err := client.RemoveConfig(key); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s reset to default.\n", key)
		return
	}
	if err := client.SetConfig(key, value); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s set to %s\n", key, value)
}

// ConfigGet prints the effective value for a configuration key.
func ConfigGet(key string) {
	value, err := client.GetConfig(key)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(value)
}
