package r2_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"codesfer/pkg/object"
	"codesfer/pkg/r2"

	"github.com/gnitoahc/go-dotenv"
)

func TestR2(t *testing.T) {
	ctx := context.Background()
	dotenv.Load("../../.env")

	accountID := os.Getenv("CF_ACCOUNT_ID")
	accessKey := os.Getenv("CF_ACCESS_KEY")
	secretKey := os.Getenv("CF_SECRET_ACCESS_KEY")
	bucket := os.Getenv("CF_BUCKET")

	if accountID == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("CF_* environment variables not set; skipping R2 integration test")
	}

	storage := r2.Storage{}
	if err := storage.Init(ctx, r2.Config{
		AccountID:       accountID,
		AccessKey:       accessKey,
		SecretAccessKey: secretKey,
		Bucket:          bucket,
	}); err != nil {
		t.Fatalf("init storage: %v", err)
	}

	ObjectImplements(t, ctx, &storage)
}

func ObjectImplements(t *testing.T, ctx context.Context, obj object.ObjectStorage) {
	t.Helper()
	t.Cleanup(func() { _ = obj.Close(ctx) })

	key := fmt.Sprintf("codesfer-test-%d.txt", time.Now().UnixNano())
	content := []byte("Hello, R2! This is a test payload.")
	meta := map[string]string{"owner": "codesfer-tests", "purpose": "integration"}

	putObj, err := obj.Put(ctx, key, bytes.NewReader(content), int64(len(content)), "text/plain", meta)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if putObj.Key != key {
		t.Fatalf("Put: expected key %s got %s", key, putObj.Key)
	}
	if putObj.Size != int64(len(content)) {
		t.Fatalf("Put: expected size %d got %d", len(content), putObj.Size)
	}

	statObj, err := obj.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if statObj.Key != key {
		t.Fatalf("Stat: expected key %s got %s", key, statObj.Key)
	}
	if statObj.Size != int64(len(content)) {
		t.Fatalf("Stat: expected size %d got %d", len(content), statObj.Size)
	}
	for k, v := range meta {
		if got := statObj.CustomMeta[k]; got != v {
			t.Fatalf("Stat: expected meta %s=%s got %s", k, v, got)
		}
	}

	gotObj, rc, err := obj.Get(ctx, key, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Get read: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("Get: content mismatch, got %q want %q", string(data), string(content))
	}
	if gotObj.Size != int64(len(content)) {
		t.Fatalf("Get: expected size %d got %d", len(content), gotObj.Size)
	}

	if err := obj.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := obj.Stat(ctx, key); !errors.Is(err, object.ErrNotFound) {
		t.Fatalf("Stat after delete: expected ErrNotFound, got %v", err)
	}
}
