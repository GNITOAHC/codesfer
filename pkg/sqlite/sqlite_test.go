package sqlite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"codesfer/pkg/object"
)

func newTestStorage(t *testing.T, allowOverwrite bool) *Storage {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "objects.db")
	src := fmt.Sprintf("file:%s?cache=shared&mode=rwc", dbPath)

	st := &Storage{}
	if err := st.Init(ctx, Config{
		Source:         src,
		AllowOverwrite: allowOverwrite,
	}); err != nil {
		t.Fatalf("init storage: %v", err)
	}
	t.Cleanup(func() { _ = st.Close(ctx) })
	return st
}

func TestSQLiteObjectStorage(t *testing.T) {
	ctx := context.Background()
	st := newTestStorage(t, true)

	key := "unit-test-key"
	content := []byte("abcdefghijklmnopqrstuvwxyz")
	meta := map[string]string{"owner": "unit-test", "purpose": "object-storage"}
	contentType := "text/plain"

	putObj, err := st.Put(ctx, key, bytes.NewReader(content), int64(len(content)), contentType, meta)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if putObj.Key != key {
		t.Fatalf("Put: expected key %s got %s", key, putObj.Key)
	}
	if putObj.Size != int64(len(content)) {
		t.Fatalf("Put: expected size %d got %d", len(content), putObj.Size)
	}
	if putObj.ContentType != contentType {
		t.Fatalf("Put: expected content type %s got %s", contentType, putObj.ContentType)
	}
	for k, v := range meta {
		if putObj.CustomMeta[k] != v {
			t.Fatalf("Put: expected meta %s=%s got %s", k, v, putObj.CustomMeta[k])
		}
	}

	statObj, err := st.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if statObj.Size != putObj.Size || statObj.ETag != putObj.ETag {
		t.Fatalf("Stat: metadata mismatch, got size %d etag %s", statObj.Size, statObj.ETag)
	}

	gotObj, rc, err := st.Get(ctx, key, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Get read: %v", err)
	}
	if string(body) != string(content) {
		t.Fatalf("Get: content mismatch, got %q want %q", string(body), string(content))
	}
	if gotObj.Size != int64(len(content)) {
		t.Fatalf("Get: expected size %d got %d", len(content), gotObj.Size)
	}

	rng := &object.Range{Start: 5, End: 9}
	_, rangeRC, err := st.Get(ctx, key, rng)
	if err != nil {
		t.Fatalf("Get range: %v", err)
	}
	defer rangeRC.Close()
	rangeData, err := io.ReadAll(rangeRC)
	if err != nil {
		t.Fatalf("Get range read: %v", err)
	}
	if string(rangeData) != "fghij" {
		t.Fatalf("Get range: expected %q got %q", "fghij", string(rangeData))
	}

	if err := st.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := st.Stat(ctx, key); err == nil || !errors.Is(err, object.ErrNotFound) {
		t.Fatalf("Stat after delete: expected ErrNotFound got %v", err)
	}
}

func TestSQLiteMultipartPut(t *testing.T) {
	ctx := context.Background()
	st := newTestStorage(t, true)

	key := "multipart-key"
	content := bytes.Repeat([]byte("a"), 256*1024) // 256 KB

	putObj, err := st.MultipartPut(ctx, key, bytes.NewReader(content), 64*1024, map[string]string{"mode": "multipart"})
	if err != nil {
		t.Fatalf("MultipartPut: %v", err)
	}
	if putObj.Size != int64(len(content)) {
		t.Fatalf("MultipartPut: expected size %d got %d", len(content), putObj.Size)
	}

	_, rc, err := st.Get(ctx, key, nil)
	if err != nil {
		t.Fatalf("Get after multipart: %v", err)
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Get read after multipart: %v", err)
	}
	if len(body) != len(content) {
		t.Fatalf("Get after multipart: size mismatch, got %d want %d", len(body), len(content))
	}
}

func TestSQLiteConflict(t *testing.T) {
	ctx := context.Background()
	st := newTestStorage(t, false) // no overwrite

	key := "conflict-key"
	content := []byte("first")

	if _, err := st.Put(ctx, key, bytes.NewReader(content), int64(len(content)), "", nil); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	if _, err := st.Put(ctx, key, bytes.NewReader([]byte("second")), -1, "", nil); !errors.Is(err, object.ErrConflict) {
		t.Fatalf("second Put: expected ErrConflict got %v", err)
	}
}
