package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func setupIndexDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "index.db")
	source := fmt.Sprintf("file:%s?cache=shared&mode=rwc", dbPath)
	if err := connect("sqlite", source); err != nil {
		t.Fatalf("connect index db: %v", err)
	}
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
			db = nil
		}
	})
}

func TestUpsertRejectsCrossUserKeyOverwrite(t *testing.T) {
	setupIndexDB(t)

	if err := insert("shared", "alice", "alice.zip", "", "alice/alice.zip", "{}", ScopePublic); err != nil {
		t.Fatalf("insert alice object: %v", err)
	}

	err := upsert("shared", "bob", "bob.zip", "", "bob/bob.zip", "{}", ScopePublic)
	if !errors.Is(err, ErrKeyOwnedByAnotherUser) {
		t.Fatalf("upsert cross-user key: got %v want ErrKeyOwnedByAnotherUser", err)
	}

	obj, err := get("shared")
	if err != nil {
		t.Fatalf("get shared key: %v", err)
	}
	if obj == nil {
		t.Fatal("shared key was deleted")
	}
	if obj.Username != "alice" || obj.IdxPath != "alice.zip" || obj.ObjPath != "alice/alice.zip" {
		t.Fatalf("shared key changed: got user=%q filename=%q path=%q", obj.Username, obj.IdxPath, obj.ObjPath)
	}
}

func TestUpsertAllowsSameUserKeyOverwrite(t *testing.T) {
	setupIndexDB(t)

	if err := insert("shared", "alice", "old.zip", "", "alice/old.zip", "{}", ScopePublic); err != nil {
		t.Fatalf("insert alice object: %v", err)
	}

	if err := upsert("shared", "alice", "new.zip", "secret", "alice/new.zip", `{"desc":"updated"}`, ScopePublic); err != nil {
		t.Fatalf("upsert same-user key: %v", err)
	}

	obj, err := get("shared")
	if err != nil {
		t.Fatalf("get shared key: %v", err)
	}
	if obj == nil {
		t.Fatal("shared key missing")
	}
	if obj.Username != "alice" || obj.IdxPath != "new.zip" || obj.Password != "secret" || obj.ObjPath != "alice/new.zip" {
		t.Fatalf("shared key not updated: got user=%q filename=%q password=%q path=%q", obj.Username, obj.IdxPath, obj.Password, obj.ObjPath)
	}
}
