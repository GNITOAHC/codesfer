package server

import (
	"context"
	"github.com/gnitoahc/codesfer/internal/server/storage"
	"github.com/gnitoahc/codesfer/pkg/cron"
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
