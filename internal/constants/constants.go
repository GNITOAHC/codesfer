// Package constants contains values shared by Codesfer's internal client,
// command-line interface, and server packages.
package constants

const (
	// UploadChunkSize stays below Cloudflare's 100 MB request-body limit.
	UploadChunkSize = 90 << 20
)
