// Package constants contains values shared by Codesfer's internal client,
// command-line interface, and server packages.
package constants

import "time"

const (
	// UploadChunkSize stays below Cloudflare's 100 MB request-body limit.
	UploadChunkSize = 90 << 20

	// ChunkSessionTTL is how long a chunked upload may sit untouched before the
	// server abandons it and releases its backend multipart (R2 bills for open
	// ones). The clock resets on every chunk, so it caps idle time, not total
	// upload time: an upload of any duration is safe as long as no gap between
	// two chunks exceeds this. Single source of truth for the sweep.
	ChunkSessionTTL = 5 * time.Hour
)
