// Package testdb picks the database tests run against: sqlite by default,
// postgres when TEST_PG_SOURCE is set.
package testdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Open returns a driver and source for an empty, test-scoped database.
// On postgres each test gets its own schema, dropped on cleanup.
func Open(t *testing.T) (driver, source string) {
	t.Helper()

	pg := os.Getenv("TEST_PG_SOURCE")
	if pg == "" {
		return "sqlite", fmt.Sprintf("file:%s?cache=shared&mode=rwc", filepath.Join(t.TempDir(), "test.db"))
	}

	admin, err := sql.Open("pgx", pg)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
		admin.Close()
	})

	sep := "?"
	if strings.Contains(pg, "?") {
		sep = "&"
	}
	return "pgx", pg + sep + "search_path=" + schema
}
