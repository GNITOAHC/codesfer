package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const tableName = "object_indices"

var db *sql.DB

var (
	ErrKeyOwnedByAnotherUser = errors.New("key already belongs to another user")
	ErrKeyTaken              = errors.New("key already in use")
	ErrPathTaken             = errors.New("filename already in use")
)

// Access scopes for an object.
const (
	ScopeOwner         = "owner"
	ScopeAuthenticated = "authenticated"
	ScopePublic        = "public"
)

type Object struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	// Uploaded object's path in indexdb
	IdxPath  string `json:"idx_path"`
	Password string `json:"password"`
	// ObjPath in object storage (unique), not same as IdxPath; this is the actual path where the object is stored
	// It's immutable once created
	ObjPath     string `json:"obj_path"`
	CreatedAt   int64  `json:"created_at"`
	Metadata    string `json:"metadata"`
	AccessScope string `json:"access_scope"`
	ExpiresAt   int64  `json:"expires_at"` // unix seconds; 0 = never expires
}

func connect(driver, source string) error {
	_db, err := sql.Open(driver, source)
	if err != nil {
		panic(err)
	}

	db = _db
	return createTable()
}

func createTable() error {
	query := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s (
            id VARCHAR(255) NOT NULL PRIMARY KEY,
			username VARCHAR(255) NOT NULL,
			idx_path VARCHAR(255),                        -- Object's index path, directory is separated by slashes
			password VARCHAR(255),
            obj_path VARCHAR(255) UNIQUE,                 -- Path in object storage
            created_at BIGINT,
			metadata TEXT,                                -- JSON string for additional metadata
			access_scope VARCHAR(16) NOT NULL DEFAULT 'public', -- owner | authenticated | public
			expires_at BIGINT,                            -- unix seconds; NULL = never expires
            UNIQUE (username, idx_path)
	)`, tableName)

	_, err := db.Exec(query)
	return err
}

// normalizeScope maps the client-provided `access` form value to a scope.
// Empty means public: uploads that don't ask for gating stay openly shareable.
func normalizeScope(s string) (string, error) {
	switch s {
	case "":
		return ScopePublic, nil
	case ScopeOwner, ScopeAuthenticated, ScopePublic:
		return s, nil
	}
	return "", fmt.Errorf("invalid access scope: %q (want owner|authenticated|public)", s)
}

func show(username string) ([]Object, error) {
	query := fmt.Sprintf("SELECT id, username, idx_path, password, obj_path, created_at, access_scope FROM %s WHERE username = $1 ORDER BY created_at DESC", tableName)
	rows, err := db.Query(query, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var objs []Object
	for rows.Next() {
		obj := Object{}
		err := rows.Scan(&obj.ID, &obj.Username, &obj.IdxPath, &obj.Password, &obj.ObjPath, &obj.CreatedAt, &obj.AccessScope)
		if err != nil {
			return nil, err
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

// nullExpiry stores expiresAt 0 as NULL (never expires).
func nullExpiry(expiresAt int64) sql.NullInt64 {
	return sql.NullInt64{Int64: expiresAt, Valid: expiresAt > 0}
}

func insert(id, user, idxPath, password, path, metadata, scope string, expiresAt int64) error {
	query := fmt.Sprintf("INSERT INTO %s (id, username, idx_path, password, obj_path, created_at, metadata, access_scope, expires_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)", tableName)
	_, err := db.Exec(query, id, user, idxPath, password, path, time.Now().Unix(), metadata, scope, nullExpiry(expiresAt))
	return err
}

// upsert will overwrite an existing record only when it belongs to the same user.
func upsert(id, user, idxPath, password, path, metadata, scope string, expiresAt int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var owner string
	err = tx.QueryRow(fmt.Sprintf("SELECT username FROM %s WHERE id = $1", tableName), id).Scan(&owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && owner != user {
		return fmt.Errorf("%w: %s", ErrKeyOwnedByAnotherUser, id)
	}

	_, err = tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = $1 AND username = $2", tableName), id, user)
	if err != nil {
		return err
	}

	// Insert new record
	query := fmt.Sprintf("INSERT INTO %s (id, username, idx_path, password, obj_path, created_at, metadata, access_scope, expires_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)", tableName)
	_, err = tx.Exec(query, id, user, idxPath, password, path, time.Now().Unix(), metadata, scope, nullExpiry(expiresAt))
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func haveFile(username, idxPath string) (bool, error) {
	query := fmt.Sprintf("SELECT id FROM %s WHERE username = $1 AND idx_path = $2", tableName)
	row := db.QueryRow(query, username, idxPath)
	var id string
	if err := row.Scan(&id); err != nil {
		if err.Error() == "sql: no rows in result set" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// objectColumns and liveObject are shared by the single-object lookups, so an
// expired object reads as missing even before the sweep deletes it.
const (
	objectColumns = "id, username, idx_path, password, obj_path, created_at, COALESCE(metadata, ''), access_scope, COALESCE(expires_at, 0)"
	liveObject    = "(expires_at IS NULL OR expires_at > $1)"
)

// scanObject reads one objectColumns row, or nil when there is none.
func scanObject(row *sql.Row) (*Object, error) {
	obj := &Object{}
	err := row.Scan(&obj.ID, &obj.Username, &obj.IdxPath, &obj.Password, &obj.ObjPath, &obj.CreatedAt, &obj.Metadata, &obj.AccessScope, &obj.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func get(id string) (*Object, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s AND id = $2", objectColumns, tableName, liveObject)
	return scanObject(db.QueryRow(query, time.Now().Unix(), id))
}

// keyExists reports whether id is taken, expired or not: an expired row keeps
// its primary key until the sweep removes it.
func keyExists(id string) (bool, error) {
	var n int
	err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = $1", tableName), id).Scan(&n)
	return n > 0, err
}

// updateObject rewrites the mutable settings (id, idx_path, metadata,
// access_scope) of the object currently stored under oldID. The username
// guard prevents updating someone else's object.
func updateObject(oldID string, obj *Object) error {
	query := fmt.Sprintf("UPDATE %s SET id = $1, idx_path = $2, metadata = $3, access_scope = $4 WHERE id = $5 AND username = $6", tableName)
	res, err := db.Exec(query, obj.ID, obj.IdxPath, obj.Metadata, obj.AccessScope, oldID, obj.Username)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("object not found")
	}
	return nil
}

// removeByID removes the object with given id and returns the path in object storage
// username should be provided to prevent unauthorized removal
func removeByID(username, id string) (string, error) {
	query := fmt.Sprintf("DELETE FROM %s WHERE username = $1 AND id = $2 returning obj_path", tableName)
	var path string
	if err := db.QueryRow(query, username, id).Scan(&path); err != nil {
		return "", err
	}
	return path, nil
}

// getByUsernamePath returns the object with given username and path.
// The path here refers to the `idx_path` field that is stored in the db
func getByUsernamePath(username, path string) (*Object, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s AND username = $2 AND idx_path = $3", objectColumns, tableName, liveObject)
	return scanObject(db.QueryRow(query, time.Now().Unix(), username, path))
}

func getAllPaths() ([]string, error) {
	query := fmt.Sprintf("SELECT obj_path FROM %s ORDER BY created_at DESC", tableName)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// removeExpired deletes every expired record and returns their paths in
// object storage.
func removeExpired() ([]string, error) {
	query := fmt.Sprintf("DELETE FROM %s WHERE expires_at IS NOT NULL AND expires_at <= $1 RETURNING obj_path", tableName)
	rows, err := db.Query(query, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}
