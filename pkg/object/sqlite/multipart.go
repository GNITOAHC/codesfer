package sqlite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/gnitoahc/codesfer/pkg/object"
)

// SQLite has no native multipart protocol, so parts are staged in a temp
// directory and concatenated by CompleteMultipart. This keeps the chunked
// upload handler backend-agnostic: it always talks object.StreamingWriter.
//
// one global map, not per-Storage state — a process runs a single
// backend. Move it onto Storage if that ever stops being true.
var (
	multipartMu   sync.Mutex
	multipartDirs = map[string]string{}
)

// CreateMultipart begins a staged upload and returns its ID.
func (s *Storage) CreateMultipart(_ context.Context, key string, _ map[string]string) (string, error) {
	if err := s.ensureDB(); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "sqlite_multipart_")
	if err != nil {
		return "", fmt.Errorf("sqlite: create multipart dir: %w", err)
	}
	uploadID := filepath.Base(dir)

	multipartMu.Lock()
	multipartDirs[uploadID] = dir
	multipartMu.Unlock()
	return uploadID, nil
}

// UploadPart stages one part. Parts may arrive in any order.
func (s *Storage) UploadPart(_ context.Context, _, uploadID string, partNumber int32, body io.Reader, _ int64) (string, error) {
	dir, err := multipartDir(uploadID)
	if err != nil {
		return "", err
	}
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("part_%d", partNumber)))
	if err != nil {
		return "", fmt.Errorf("sqlite: create part: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, body); err != nil {
		return "", fmt.Errorf("sqlite: write part: %w", err)
	}
	// ETag is unused by this backend; the part number keeps it unique.
	return fmt.Sprintf("part-%d", partNumber), nil
}

// CompleteMultipart concatenates the staged parts in part-number order and
// stores the result under key.
func (s *Storage) CompleteMultipart(ctx context.Context, key, uploadID string, parts []object.CompletedPart) (object.Object, error) {
	dir, err := multipartDir(uploadID)
	if err != nil {
		return object.Object{}, err
	}
	defer s.AbortMultipart(ctx, key, uploadID) //nolint:errcheck — best-effort cleanup

	ordered := make([]object.CompletedPart, len(parts))
	copy(ordered, parts)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].PartNumber < ordered[j].PartNumber })

	pr, pw := io.Pipe()
	go func() {
		for _, p := range ordered {
			f, err := os.Open(filepath.Join(dir, fmt.Sprintf("part_%d", p.PartNumber)))
			if err != nil {
				pw.CloseWithError(err)
				return
			}
			_, copyErr := io.Copy(pw, f)
			f.Close()
			if copyErr != nil {
				pw.CloseWithError(copyErr)
				return
			}
		}
		pw.Close()
	}()
	defer pr.Close()

	return s.save(ctx, key, pr, "", nil)
}

// AbortMultipart discards the staged parts.
func (s *Storage) AbortMultipart(_ context.Context, _, uploadID string) error {
	multipartMu.Lock()
	dir, ok := multipartDirs[uploadID]
	delete(multipartDirs, uploadID)
	multipartMu.Unlock()
	if !ok {
		return nil
	}
	return os.RemoveAll(dir)
}

func multipartDir(uploadID string) (string, error) {
	multipartMu.Lock()
	defer multipartMu.Unlock()
	dir, ok := multipartDirs[uploadID]
	if !ok {
		return "", errors.New("sqlite: unknown multipart upload: " + uploadID)
	}
	return dir, nil
}
