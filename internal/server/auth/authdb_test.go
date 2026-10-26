package auth

import (
	"testing"

	"github.com/gnitoahc/codesfer/internal/testdb"
)

func setupAuthDB(t *testing.T) {
	t.Helper()
	if err := connect(testdb.Open(t)); err != nil {
		t.Fatalf("connect auth db: %v", err)
	}
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
			db = nil
		}
	})
}

// exerciseAuthDB runs every auth query against the connected db.
func exerciseAuthDB(t *testing.T) {
	t.Helper()

	if err := createUser("a@test.com", "pw", "alice"); err != nil {
		t.Fatalf("createUser: %v", err)
	}
	if err := createUser("a@test.com", "pw", "alice"); err != ErrUserAlreadyExists {
		t.Fatalf("duplicate createUser: got %v want ErrUserAlreadyExists", err)
	}
	if _, err := getUser("nobody@test.com"); err != ErrUserNotFound {
		t.Fatalf("getUser missing: got %v want ErrUserNotFound", err)
	}
	if !usernameExists("alice") || usernameExists("bob") {
		t.Fatal("usernameExists wrong")
	}
	if ok, err := verify("a@test.com", "pw"); err != nil || !ok {
		t.Fatalf("verify good password: %v %v", ok, err)
	}
	if ok, _ := verify("a@test.com", "bad"); ok {
		t.Fatal("verify accepted bad password")
	}

	sid, err := createSession("a@test.com", "agent", "127.0.0.1")
	if err != nil {
		t.Fatalf("createSession: %v", err)
	}
	user, err := getUserFromSessionID(sid)
	if err != nil || user.Username != "alice" {
		t.Fatalf("getUserFromSessionID: %+v %v", user, err)
	}
	if err := updateSessionLastSeen(sid); err != nil {
		t.Fatalf("updateSessionLastSeen: %v", err)
	}
	sessions, err := getSessions("a@test.com")
	if err != nil || len(sessions) != 1 || sessions[0].LastSeen == 0 {
		t.Fatalf("getSessions: %+v %v", sessions, err)
	}
	if err := deleteSessionByName("a@test.com", "no-such-name"); err == nil {
		t.Fatal("deleteSessionByName missing name: want error")
	}
	if err := deleteSessionByName("a@test.com", sessions[0].Name); err != nil {
		t.Fatalf("deleteSessionByName: %v", err)
	}

	sid, err = createSession("a@test.com", "agent", "127.0.0.1")
	if err != nil {
		t.Fatalf("createSession: %v", err)
	}
	if err := deleteSession(sid); err != nil {
		t.Fatalf("deleteSession: %v", err)
	}
	if _, err := getSession(sid); err == nil {
		t.Fatal("getSession after delete: want error")
	}
}

func TestAuthDB(t *testing.T) {
	setupAuthDB(t)
	exerciseAuthDB(t)
}
