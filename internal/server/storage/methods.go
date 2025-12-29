package storage

import (
	"context"
	"log"
)

func RemoveOrphanedObject() error {
	log.Println("Removing orphaned objects...")
	indexed, err := getAllPaths()
	if err != nil {
		return err
	}
	objects, err := objectStorage.List(context.Background(), "")
	if err != nil {
		return err
	}

	// Create a map for O(1) lookup of indexed paths
	indexedMap := make(map[string]struct{}, len(indexed))
	for _, path := range indexed {
		log.Printf("Indexed path: %s", path)
		indexedMap[path] = struct{}{}
	}

	var orphans []string
	for _, obj := range objects {
        log.Printf("Object: %s", obj.Key)
		if _, exists := indexedMap[obj.Key]; !exists {
			orphans = append(orphans, obj.Key)
		}
	}

	log.Printf("Found %d orphaned objects", len(orphans))
	for _, orphan := range orphans {
		log.Printf("Removing orphaned object: %s", orphan)
		if err := objectStorage.Delete(context.Background(), orphan); err != nil {
			log.Printf("Failed to remove orphaned object %s: %v", orphan, err)
		}
	}

	return nil
}
