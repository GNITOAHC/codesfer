// Package sqlite implements object.ObjectStorage backed by SQLite.
package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"strings"
	"time"

	"codesfer/pkg/object"

	_ "modernc.org/sqlite"
)

// Config defines how the SQLite storage should be initialized.
type Config struct {
	// Source is the DSN/connection string, e.g. file:objects.db?cache=shared.
	Source string
	// Driver name registered with database/sql. Defaults to "sqlite".
	Driver string
	// Table to store objects. Defaults to "objects".
	Table string
	// AllowOverwrite controls whether Put replaces existing records (default true).
	AllowOverwrite bool
	// DB lets callers supply an existing *sql.DB connection.
	DB *sql.DB
}

// Storage satisfies object.ObjectStorage using a SQLite table.
type Storage struct {
	db             *sql.DB
	table          string
	allowOverwrite bool
	ownsDB         bool
}

// Init configures the storage and ensures the backing table exists.
func (s *Storage) Init(ctx context.Context, param any) error {
	cfg, ok := param.(Config)
	if !ok {
		if p, ok := param.(*Config); ok && p != nil {
			cfg = *p
		} else {
			return fmt.Errorf("sqlite: unexpected config type %T", param)
		}
	}

	if cfg.Driver == "" {
		cfg.Driver = "sqlite"
	}
	if cfg.Table == "" {
		cfg.Table = "objects"
	}
	if cfg.Source == "" && cfg.DB == nil {
		return errors.New("sqlite: Source is required")
	}

	table, err := sanitizeName(cfg.Table)
	if err != nil {
		return err
	}
	s.table = table
	s.allowOverwrite = cfg.AllowOverwrite

	if cfg.DB != nil {
		s.db = cfg.DB
	} else {
		db, err := sql.Open(cfg.Driver, cfg.Source)
		if err != nil {
			return fmt.Errorf("sqlite: open database: %w", err)
		}
		s.db = db
		s.ownsDB = true
	}

	createStmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		key TEXT PRIMARY KEY,
		data BLOB NOT NULL,
		size INTEGER NOT NULL,
		etag TEXT,
		content_type TEXT,
		last_modified TEXT NOT NULL,
		meta TEXT
	)`, s.table)

	if _, err := s.db.ExecContext(ctx, createStmt); err != nil {
		return fmt.Errorf("sqlite: create table: %w", err)
	}

	return nil
}

// Close releases the DB connection when owned by the storage.
func (s *Storage) Close(_ context.Context) error {
	if s.db != nil && s.ownsDB {
		return s.db.Close()
	}
	return nil
}

// Put stores an object; optionally overwrites existing content based on config.
func (s *Storage) Put(ctx context.Context, key string, r io.Reader, _ int64, contentType string, meta map[string]string) (object.Object, error) {
	return s.save(ctx, key, r, contentType, meta)
}

// MultipartPut streams large uploads; stored atomically for SQLite backend.
func (s *Storage) MultipartPut(ctx context.Context, key string, r io.Reader, _ int64, meta map[string]string) (object.Object, error) {
	return s.save(ctx, key, r, "", meta)
}

// Get retrieves the object data and metadata.
func (s *Storage) Get(ctx context.Context, key string, rng *object.Range) (object.Object, io.ReadCloser, error) {
	if err := s.ensureDB(); err != nil {
		return object.Object{}, nil, err
	}

	query := fmt.Sprintf(`SELECT size, etag, content_type, last_modified, meta, data FROM %s WHERE key = ?`, s.table)
	var (
		size         int64
		etag         sql.NullString
		contentType  sql.NullString
		lastModified string
		metaJSON     sql.NullString
		data         []byte
	)

	err := s.db.QueryRowContext(ctx, query, key).Scan(&size, &etag, &contentType, &lastModified, &metaJSON, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return object.Object{}, nil, object.ErrNotFound
	}
	if err != nil {
		return object.Object{}, nil, fmt.Errorf("sqlite: get object: %w", err)
	}

	obj, err := s.rowToObject(key, size, etag.String, contentType.String, lastModified, metaJSON.String)
	if err != nil {
		return object.Object{}, nil, err
	}

	slice, err := applyRange(data, rng)
	if err != nil {
		return object.Object{}, nil, err
	}

	return obj, io.NopCloser(bytes.NewReader(slice)), nil
}

// List returns all objects with the given prefix.
func (s *Storage) List(ctx context.Context, prefix string) ([]object.Object, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT key, size, etag, content_type, last_modified, meta FROM %s WHERE key LIKE ? ORDER BY key ASC`, s.table)

	// Note: We are not escaping % and _ in the prefix, so they will be treated as wildcards.
	// This is acceptable for a basic implementation.
	rows, err := s.db.QueryContext(ctx, query, prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("sqlite: list objects: %w", err)
	}
	defer rows.Close()

	var objects []object.Object
	for rows.Next() {
		var (
			key          string
			size         int64
			etag         sql.NullString
			contentType  sql.NullString
			lastModified string
			metaJSON     sql.NullString
		)
		if err := rows.Scan(&key, &size, &etag, &contentType, &lastModified, &metaJSON); err != nil {
			return nil, fmt.Errorf("sqlite: scan object: %w", err)
		}

		obj, err := s.rowToObject(key, size, etag.String, contentType.String, lastModified, metaJSON.String)
		if err != nil {
			return nil, err
		}
		objects = append(objects, obj)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate objects: %w", err)
	}

	return objects, nil
}

// Stat fetches metadata without streaming the body.
func (s *Storage) Stat(ctx context.Context, key string) (object.Object, error) {
	if err := s.ensureDB(); err != nil {
		return object.Object{}, err
	}

	query := fmt.Sprintf(`SELECT size, etag, content_type, last_modified, meta FROM %s WHERE key = ?`, s.table)
	var (
		size         int64
		etag         sql.NullString
		contentType  sql.NullString
		lastModified string
		metaJSON     sql.NullString
	)

	err := s.db.QueryRowContext(ctx, query, key).Scan(&size, &etag, &contentType, &lastModified, &metaJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return object.Object{}, object.ErrNotFound
	}
	if err != nil {
		return object.Object{}, fmt.Errorf("sqlite: stat object: %w", err)
	}

	return s.rowToObject(key, size, etag.String, contentType.String, lastModified, metaJSON.String)
}

// Delete removes an object by key.
func (s *Storage) Delete(ctx context.Context, key string) error {
	if err := s.ensureDB(); err != nil {
		return err
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE key = ?`, s.table)
	res, err := s.db.ExecContext(ctx, query, key)
	if err != nil {
		return fmt.Errorf("sqlite: delete object: %w", err)
	}

	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return object.ErrNotFound
	}
	return err
}

func (s *Storage) save(ctx context.Context, key string, r io.Reader, contentType string, meta map[string]string) (object.Object, error) {
	if err := s.ensureDB(); err != nil {
		return object.Object{}, err
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return object.Object{}, fmt.Errorf("sqlite: read content: %w", err)
	}

	metaJSON, err := encodeMeta(meta)
	if err != nil {
		return object.Object{}, err
	}

	now := time.Now().UTC()
	obj := object.Object{
		Key:          key,
		Size:         int64(len(data)),
		ETag:         hashETag(data),
		ContentType:  contentType,
		LastModified: now,
		CustomMeta:   cloneMeta(meta),
	}

	query := fmt.Sprintf(`INSERT INTO %s (key, data, size, etag, content_type, last_modified, meta) VALUES (?, ?, ?, ?, ?, ?, ?)`, s.table)
	if s.allowOverwrite {
		query += ` ON CONFLICT(key) DO UPDATE SET data=excluded.data, size=excluded.size, etag=excluded.etag, content_type=excluded.content_type, last_modified=excluded.last_modified, meta=excluded.meta`
	}

	_, err = s.db.ExecContext(ctx, query,
		key,
		data,
		obj.Size,
		nullIfEmpty(obj.ETag),
		nullIfEmpty(contentType),
		now.Format(time.RFC3339Nano),
		nullIfEmpty(metaJSON),
	)
	if err != nil {
		if isConflict(err) {
			return object.Object{}, object.ErrConflict
		}
		return object.Object{}, fmt.Errorf("sqlite: put object: %w", err)
	}

	return obj, nil
}

func (s *Storage) ensureDB() error {
	if s.db == nil {
		return errors.New("sqlite: storage not initialized")
	}
	return nil
}

func (s *Storage) rowToObject(key string, size int64, etag, contentType, lastModified, metaJSON string) (object.Object, error) {
	t, err := time.Parse(time.RFC3339Nano, lastModified)
	if err != nil {
		return object.Object{}, fmt.Errorf("sqlite: parse last_modified: %w", err)
	}

	metaMap, err := decodeMeta(metaJSON)
	if err != nil {
		return object.Object{}, err
	}

	return object.Object{
		Key:          key,
		Size:         size,
		ETag:         etag,
		ContentType:  contentType,
		LastModified: t,
		CustomMeta:   metaMap,
	}, nil
}

func applyRange(data []byte, rng *object.Range) ([]byte, error) {
	if rng == nil {
		return data, nil
	}
	if rng.Start < 0 {
		return nil, fmt.Errorf("sqlite: invalid range start %d", rng.Start)
	}
	if rng.Start >= int64(len(data)) {
		return nil, fmt.Errorf("sqlite: range start beyond object size")
	}
	end := rng.End
	if end < 0 || end >= int64(len(data)) {
		end = int64(len(data) - 1)
	}
	if end < rng.Start {
		return nil, fmt.Errorf("sqlite: invalid range end %d", rng.End)
	}
	return data[rng.Start : end+1], nil
}

func hashETag(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func encodeMeta(meta map[string]string) (string, error) {
	if len(meta) == 0 {
		return "", nil
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("sqlite: marshal metadata: %w", err)
	}
	return string(b), nil
}

func decodeMeta(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("sqlite: unmarshal metadata: %w", err)
	}
	return out, nil
}

func sanitizeName(name string) (string, error) {
	re := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	if !re.MatchString(name) {
		return "", fmt.Errorf("sqlite: invalid table name %q", name)
	}
	return name, nil
}

func isConflict(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "constraint failed")
}

func cloneMeta(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Ensure Storage implements ObjectStorage interface.
var _ object.ObjectStorage = (*Storage)(nil)
