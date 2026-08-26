package server

import (
	"context"
	"time"

	"github.com/gnitoahc/codesfer/internal/constants"
	"github.com/gnitoahc/codesfer/internal/server/storage"
	"github.com/gnitoahc/codesfer/pkg/cron"
)

// 1. Clean up expired sessions (wip)
// 2. Remove orphaned objects
// 3. Abort abandoned chunked uploads
func AddCronJobs(cronMgr *cron.Manager) {
	jobCleanObjs := func(ctx context.Context) error {
		return storage.RemoveOrphanedObject()
	}
	cronMgr.Add("clean-orphaned-objects", 31*24*time.Hour, jobCleanObjs)

	jobSweepChunks := func(ctx context.Context) error {
		return storage.SweepStaleChunkSessions(ctx, constants.ChunkSessionTTL)
	}
	// The interval only decides how promptly an abandoned upload is released;
	// the sweep is safe to run at any frequency, so keep it well under the TTL.
	cronMgr.Add("sweep-chunk-sessions", time.Hour, jobSweepChunks)
}
