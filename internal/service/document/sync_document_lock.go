package document

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/service"
)

// acquireSyncDocumentLock serializes sync side effects across MySQL sessions,
// including for documents that have not been inserted yet. SQLite uses the
// process lock only for the default in-memory unit fixtures.
func acquireSyncDocumentLock(ctx context.Context, docID string) (func(), error) {
	key := "sync-document:" + service.Hash128(docID)
	if dao.DB.Dialector.Name() == "sqlite" {
		// SQLite is used by the in-memory service unit fixtures. Its lock key
		// must differ from the parse lock acquired by StartParseDocuments.
		return lockDocumentParse(key), nil
	}
	pool, err := dao.OpenIndependentMySQLDB(dao.DB)
	if err != nil {
		return nil, err
	}
	unlock, err := acquireMySQLSyncDocumentLock(ctx, pool, key)
	if err != nil {
		_ = pool.Close()
		return nil, err
	}
	return func() {
		// Closing the dedicated session also releases a named lock if the
		// explicit release fails; it is never returned to the application pool.
		defer pool.Close()
		unlock()
	}, nil
}

// acquireMySQLSyncDocumentLock owns a named lock on one dedicated connection
// until its release callback runs. It holds no row lock or SQL transaction.
func acquireMySQLSyncDocumentLock(ctx context.Context, pool *sql.DB, key string) (func(), error) {
	lockCtx, cancel := context.WithTimeout(ctx, cleanupBatchTimeout)
	defer cancel()
	conn, err := pool.Conn(lockCtx)
	if err != nil {
		return nil, err
	}
	var granted sql.NullInt64
	err = conn.QueryRowContext(lockCtx, "SELECT GET_LOCK(?, ?)", key, int64(cleanupBatchTimeout/time.Second)).Scan(&granted)
	if err != nil || !granted.Valid || granted.Int64 != 1 {
		_ = conn.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("could not acquire sync document lock %s", key)
	}
	return func() {
		defer conn.Close()
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupBatchTimeout)
		defer cancel()
		var released sql.NullInt64
		if err := conn.QueryRowContext(releaseCtx, "SELECT RELEASE_LOCK(?)", key).Scan(&released); err != nil || !released.Valid || released.Int64 != 1 {
			common.Warn(fmt.Sprintf("release sync document lock %s failed: %v", key, err))
		}
	}, nil
}
