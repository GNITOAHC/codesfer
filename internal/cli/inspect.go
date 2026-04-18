package cli

import (
	"encoding/json"
	"fmt"
	"github.com/gnitoahc/codesfer/internal/client"
	"log"
	"sort"
	"strings"
	"time"
)

type InspectFlags struct {
	Pass  string
	JSON  bool
	Level int
}

func Inspect(flags InspectFlags, key string) {
	sessionID := client.ReadSessionID()

	info, err := client.Inspect(sessionID, key, flags.Pass)
	if err != nil {
		log.Fatalf("Inspect failed: %v", err)
	}

	// JSON output mode - output raw metadata
	if flags.JSON {
		output, _ := json.MarshalIndent(info.Metadata, "", "  ")
		fmt.Println(string(output))
		return
	}

	// Standard output
	fmt.Printf("Key: %s\n", info.Key)
	fmt.Printf("Owner: %s\n", info.Owner)
	fmt.Printf("Path: %s\n", info.Path)
	fmt.Printf("Created: %s\n", time.Unix(info.CreatedAt, 0).Format("2006-01-02 15:04:05"))
	if info.Protected {
		fmt.Printf("Protected: yes\n")
	}

	if info.Metadata != nil {
		if desc, ok := info.Metadata["desc"]; ok && desc != "" {
			fmt.Printf("\nDescription: %s\n", desc)
		}
		if tree, ok := info.Metadata["tree"]; ok {
			fmt.Printf("\nFiles:\n")
			if files, ok := tree.([]any); ok {
				printTree(files, flags.Level)
			}
		}
	}
}

// printTree renders a visual tree structure from a flat list of file paths
func printTree(files []any, maxLevel int) {
	// Convert to string slice
	paths := make([]string, 0, len(files))
	for _, f := range files {
		if s, ok := f.(string); ok {
			paths = append(paths, s)
		}
	}
	sort.Strings(paths)

	// Build tree structure
	root := &treeNode{children: make(map[string]*treeNode)}
	for _, p := range paths {
		parts := strings.Split(p, "/")
		current := root
		for i, part := range parts {
			if current.children[part] == nil {
				current.children[part] = &treeNode{
					name:     part,
					children: make(map[string]*treeNode),
				}
			}
			current = current.children[part]
			// Mark as file if it's the last part
			if i == len(parts)-1 {
				current.isFile = true
			}
		}
	}

	// Print tree with depth limit (level 1 = top-level items)
	printTreeNode(root, "", 1, maxLevel)
}

type treeNode struct {
	name     string
	isFile   bool
	children map[string]*treeNode
}

// printTreeNode recursively prints the tree structure with visual connectors.
//
// Parameters:
//   - node: the current tree node to print
//   - prefix: the prefix string for indentation (e.g., "│   " or "    ")
//   - level: the current depth in the tree (1 = top-level files/directories)
//   - maxLevel: maximum depth to display (0 = unlimited)
//
// Level examples:
//
//	scripts/          ← level 1
//	├── build.sh      ← level 2
//	├── deploy.sh     ← level 2
//	└── utils/        ← level 2
//	    └── helper.py ← level 3
//	src/              ← level 1
//	└── main.go       ← level 2
//
// With maxLevel=2, level 3+ items are truncated and shown as "...".
func printTreeNode(node *treeNode, prefix string, level, maxLevel int) {
	// Get sorted children names
	names := make([]string, 0, len(node.children))
	for name := range node.children {
		names = append(names, name)
	}
	sort.Strings(names)

	for i, name := range names {
		child := node.children[name]
		isLastChild := i == len(names)-1

		// Determine connector
		connector := "├── "
		if isLastChild {
			connector = "└── "
		}

		// Calculate new prefix for children
		newPrefix := prefix
		if isLastChild {
			newPrefix += "    "
		} else {
			newPrefix += "│   "
		}

		// Check if we should truncate at this level
		hasChildren := len(child.children) > 0
		if maxLevel > 0 && level >= maxLevel && hasChildren {
			// Show directory with "..." to indicate more content
			fmt.Printf("%s%s%s/\n", prefix, connector, name)
			fmt.Printf("%s└── ...\n", newPrefix)
			continue
		}

		// Print current node
		if hasChildren {
			fmt.Printf("%s%s%s/\n", prefix, connector, name)
			printTreeNode(child, newPrefix, level+1, maxLevel)
		} else {
			fmt.Printf("%s%s%s\n", prefix, connector, name)
		}
	}
}
