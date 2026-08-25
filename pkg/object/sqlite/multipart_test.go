package sqlite

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/gnitoahc/codesfer/pkg/object"
)

// Parts arrive out of order; the stored object must be their part-number order.
func TestSQLiteMultipartOutOfOrder(t *testing.T) {
	ctx := context.Background()
	st := newTestStorage(t, true)

	key := "multipart-key"
	uploadID, err := st.CreateMultipart(ctx, key, nil)
	if err != nil {
		t.Fatalf("CreateMultipart: %v", err)
	}

	chunks := map[int32][]byte{1: []byte("alpha"), 2: []byte("beta"), 3: []byte("gamma")}
	var parts []object.CompletedPart
	for _, n := range []int32{3, 1, 2} { // deliberately out of order
		etag, err := st.UploadPart(ctx, key, uploadID, n, bytes.NewReader(chunks[n]), int64(len(chunks[n])))
		if err != nil {
			t.Fatalf("UploadPart %d: %v", n, err)
		}
		parts = append(parts, object.CompletedPart{ETag: etag, PartNumber: n})
	}

	if _, err := st.CompleteMultipart(ctx, key, uploadID, parts); err != nil {
		t.Fatalf("CompleteMultipart: %v", err)
	}

	_, rc, err := st.Get(ctx, key, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "alphabetagamma" {
		t.Fatalf("assembled body = %q, want %q", body, "alphabetagamma")
	}

	// Completing must not leave staged parts behind.
	if _, err := multipartDir(uploadID); err == nil {
		t.Fatal("multipart dir still registered after CompleteMultipart")
	}
}

func TestSQLiteAbortMultipart(t *testing.T) {
	ctx := context.Background()
	st := newTestStorage(t, true)

	uploadID, err := st.CreateMultipart(ctx, "aborted", nil)
	if err != nil {
		t.Fatalf("CreateMultipart: %v", err)
	}
	dir, err := multipartDir(uploadID)
	if err != nil {
		t.Fatalf("multipartDir: %v", err)
	}
	if _, err := st.UploadPart(ctx, "aborted", uploadID, 1, bytes.NewReader([]byte("x")), 1); err != nil {
		t.Fatalf("UploadPart: %v", err)
	}

	if err := st.AbortMultipart(ctx, "aborted", uploadID); err != nil {
		t.Fatalf("AbortMultipart: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("temp dir %s still exists after abort (err=%v)", dir, err)
	}
	// Aborting an unknown upload is a no-op, not an error.
	if err := st.AbortMultipart(ctx, "aborted", uploadID); err != nil {
		t.Fatalf("second AbortMultipart: %v", err)
	}
}
