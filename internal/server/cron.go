package server

import (
	"codesfer/internal/server/storage"
	"codesfer/pkg/cron"
	"context"
	"time"
)

// 1. Clean up expired sessions (wip)
// 2. Remove orphaned objects
func AddCronJobs(cronMgr *cron.Manager) {
	jobCleanObjs := func(ctx context.Context) error {
		return storage.RemoveOrphanedObject()
	}
	cronMgr.Add("clean-orphaned-objects", 31*24*time.Hour, jobCleanObjs)
}
