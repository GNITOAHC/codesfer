// Package storage contains the object storage interface
// Implementation including Cloudflare R2 or SQLite
package object

import (
	"context"
	"errors"
	"io"
	"time"
)

// Object holds metadata about a stored item.
type Object struct {
	Key          string
	Size         int64
	ETag         string
	ContentType  string
	LastModified time.Time
	CustomMeta   map[string]string
}

// Range represents a byte range [Start, End] inclusive.
// If End < 0 the range is open-ended.
type Range struct {
	Start int64
	End   int64
}

// Common errors returned by implementations.
var (
	ErrNotFound = errors.New("object not found")
	ErrConflict = errors.New("object already exists")
)

// Lifecycle defines init/teardown behavior.
type Lifecycle interface {
	Init(ctx context.Context, param any) error
	Close(ctx context.Context) error
}

// Reader exposes read-related operations.
type Reader interface {
	// Get returns object metadata and a stream the caller must close.
	Get(ctx context.Context, key string, rng *Range) (Object, io.ReadCloser, error)
	// List returns a list of objects matching the prefix.
	List(ctx context.Context, prefix string) ([]Object, error)
}

// Writer exposes write-related operations.
type Writer interface {
	// Put uploads content and returns stored metadata.
	Put(ctx context.Context, key string, r io.Reader, sizeHint int64, contentType string, meta map[string]string) (Object, error)
	// MultipartPut streams large content in parts; implementations may tune part handling internally.
	MultipartPut(ctx context.Context, key string, r io.Reader, partSize int64, meta map[string]string) (Object, error)
}

// Deleter exposes delete behavior.
type Deleter interface {
	Delete(ctx context.Context, key string) error
}

// CompletedPart records the ETag and number of a successfully uploaded multipart part.
type CompletedPart struct {
	ETag       string
	PartNumber int32
}

// StreamingWriter allows backends that support a native multipart protocol to
// expose individual upload steps. Implementing this interface enables each
// client chunk to be forwarded to object storage immediately, avoiding the need
// to reassemble the full file on disk before the final upload.
type StreamingWriter interface {
	// CreateMultipart begins a multipart upload and returns the upload ID.
	CreateMultipart(ctx context.Context, key string, meta map[string]string) (uploadID string, err error)
	// UploadPart uploads one part (1-indexed partNumber) of the given size.
	UploadPart(ctx context.Context, key, uploadID string, partNumber int32, body io.Reader, size int64) (etag string, err error)
	// CompleteMultipart finalises the upload using the provided completed parts.
	CompleteMultipart(ctx context.Context, key, uploadID string, parts []CompletedPart) (Object, error)
	// AbortMultipart cancels a multipart upload and releases its resources.
	AbortMultipart(ctx context.Context, key, uploadID string) error
}

// ObjectStorage aggregates the full contract for object backends.
type ObjectStorage interface {
	Lifecycle
	Reader
	Writer
	Deleter
	// Stat returns metadata without streaming the body.
	Stat(ctx context.Context, key string) (Object, error)
}
