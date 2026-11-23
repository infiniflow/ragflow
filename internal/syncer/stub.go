//go:build !ee

package syncer

import (
	"context"

	"ragflow/internal/dao"
)

func recordTaskEvent(context.Context, *dao.SyncTaskDAO, string, int, string, string) {}
