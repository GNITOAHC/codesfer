package storage

import (
	"errors"
	"testing"

	"github.com/gnitoahc/codesfer/internal/testdb"
)

func setupIndexDB(t *testing.T) {
	t.Helper()

	if err := connect(testdb.Open(t)); err != nil {
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

	if err := insert("shared", "alice", "alice.zip", "", "alice/alice.zip", "{}", ScopePublic, 0); err != nil {
		t.Fatalf("insert alice object: %v", err)
	}

	err := upsert("shared", "bob", "bob.zip", "", "bob/bob.zip", "{}", ScopePublic, 0)
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

	if err := insert("shared", "alice", "old.zip", "", "alice/old.zip", "{}", ScopePublic, 0); err != nil {
		t.Fatalf("insert alice object: %v", err)
	}

	if err := upsert("shared", "alice", "new.zip", "secret", "alice/new.zip", `{"desc":"updated"}`, ScopePublic, 0); err != nil {
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

// TestIndexDBQueries runs every index query against the test db.
func TestIndexDBQueries(t *testing.T) {
	setupIndexDB(t)

	if err := insert("old", "alice", "old.zip", "", "alice/old.zip", "", ScopePublic, 0); err != nil {
		t.Fatalf("insert: %v", err)
	}
	obj, err := getByUsernamePath("alice", "old.zip")
	if err != nil || obj == nil || obj.CreatedAt == 0 || obj.AccessScope != ScopePublic {
		t.Fatalf("getByUsernamePath: %+v %v", obj, err)
	}
	if err := insert("new", "alice", "new.zip", "", "alice/new.zip", "{}", ScopeOwner, 0); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := upsert("new", "alice", "new.zip", "pw", "alice/new2.zip", "{}", ScopeOwner, 0); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if ok, err := haveFile("alice", "new.zip"); err != nil || !ok {
		t.Fatalf("haveFile: %v %v", ok, err)
	}
	objs, err := show("alice")
	if err != nil || len(objs) != 2 || objs[0].ID != "new" {
		t.Fatalf("show: %+v %v", objs, err)
	}
	obj.ID, obj.Metadata = "renamed", `{"desc":"x"}`
	if err := updateObject("old", obj); err != nil {
		t.Fatalf("updateObject: %v", err)
	}
	if got, err := get("renamed"); err != nil || got == nil || got.Metadata != `{"desc":"x"}` {
		t.Fatalf("get renamed: %+v %v", got, err)
	}
	if path, err := removeByID("alice", "renamed"); err != nil || path != "alice/old.zip" {
		t.Fatalf("removeByID: %q %v", path, err)
	}
	paths, err := getAllPaths()
	if err != nil || len(paths) != 1 || paths[0] != "alice/new2.zip" {
		t.Fatalf("getAllPaths: %v %v", paths, err)
	}
}
