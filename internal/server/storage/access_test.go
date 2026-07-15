package storage

import (
	"net/http"
	"testing"
)

func TestCheckAccess(t *testing.T) {
	cases := []struct {
		name               string
		obj                Object
		username, password string
		status             int
		gate               string
	}{
		{"public anonymous", Object{AccessScope: ScopePublic, Username: "alice"}, "", "", 0, ""},
		{"public password ok", Object{AccessScope: ScopePublic, Username: "alice", Password: "pw"}, "", "pw", 0, ""},
		{"public password missing", Object{AccessScope: ScopePublic, Username: "alice", Password: "pw"}, "", "", http.StatusUnauthorized, "password"},
		{"public password wrong", Object{AccessScope: ScopePublic, Username: "alice", Password: "pw"}, "", "nope", http.StatusUnauthorized, "password"},
		{"owner bypasses password", Object{AccessScope: ScopePublic, Username: "alice", Password: "pw"}, "alice", "", 0, ""},
		{"authenticated anonymous", Object{AccessScope: ScopeAuthenticated, Username: "alice"}, "", "", http.StatusUnauthorized, "auth"},
		{"authenticated logged in", Object{AccessScope: ScopeAuthenticated, Username: "alice"}, "bob", "", 0, ""},
		{"authenticated with password gate", Object{AccessScope: ScopeAuthenticated, Username: "alice", Password: "pw"}, "bob", "", http.StatusUnauthorized, "password"},
		{"owner scope anonymous", Object{AccessScope: ScopeOwner, Username: "alice"}, "", "", http.StatusUnauthorized, "auth"},
		{"owner scope other user hidden", Object{AccessScope: ScopeOwner, Username: "alice"}, "bob", "", http.StatusNotFound, ""},
		{"owner scope owner", Object{AccessScope: ScopeOwner, Username: "alice"}, "alice", "", 0, ""},
	}
	for _, c := range cases {
		status, _, gate := checkAccess(&c.obj, c.username, c.password)
		if status != c.status || gate != c.gate {
			t.Errorf("%s: got (%d, %q), want (%d, %q)", c.name, status, gate, c.status, c.gate)
		}
	}
}

func TestNormalizeScope(t *testing.T) {
	if s, err := normalizeScope(""); err != nil || s != ScopePublic {
		t.Errorf("empty: got (%q, %v), want public", s, err)
	}
	for _, valid := range []string{ScopeOwner, ScopeAuthenticated, ScopePublic} {
		if s, err := normalizeScope(valid); err != nil || s != valid {
			t.Errorf("%s: got (%q, %v)", valid, s, err)
		}
	}
	if _, err := normalizeScope("everyone"); err == nil {
		t.Error("invalid scope accepted")
	}
}
