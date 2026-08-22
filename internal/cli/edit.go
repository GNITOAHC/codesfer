package cli

import (
	"fmt"
	"log"

	"github.com/gnitoahc/codesfer/internal/client"
	"github.com/gnitoahc/codesfer/pkg/api"
)

// Edit changes the settings of an uploaded code snippet.
// Nil fields in settings are left unchanged.
func Edit(settings api.UpdateSettingsRequest, key string) {
	if settings.Key == nil && settings.IdxPath == nil && settings.Desc == nil && settings.AccessScope == nil {
		log.Fatal("nothing to change: pass at least one of --key, --path, --desc or --access")
	}

	sessionID := client.ReadSessionID()
	if sessionID == "" {
		log.Fatal("You are not logged in. Login first to edit.")
	}

	info, err := client.UpdateSettings(sessionID, key, settings)
	if err != nil {
		log.Fatalf("Edit failed: %v", err)
	}

	fmt.Printf("Key: %s\n", info.Key)
	fmt.Printf("Path: %s\n", info.Path)
	fmt.Printf("Access: %s\n", info.AccessScope)
	if desc, ok := info.Metadata["desc"]; ok && desc != "" {
		fmt.Printf("Description: %s\n", desc)
	}
}
