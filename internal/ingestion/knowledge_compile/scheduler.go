//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package knowledge_compile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrClaimTokenMismatch is returned by Ack/TouchClaim when the supplied token
// no longer identifies the live claim (another worker took over, or the
// sweeper reclaimed it). The caller must abort without writing.
var ErrClaimTokenMismatch = errors.New("knowledge_compile: claim token mismatch")

// notifySubject is the Option E wake-up channel (§11.4): publishers push a
// {dataset_id} payload here after appending to a KB's backlog; workers
// subscribe to wake promptly. It carries no routing payload — MySQL is the
// scheduling truth.
const notifySubject = "notify.kc.workers"

// Dataset compile lifecycle states. Defined in entity so the scheduler, the
// dataset service (status API) and tests share one source of truth.
const (
	DatasetStateIdle      = entity.DatasetStateIdle
	DatasetStatePending   = entity.DatasetStatePending
	DatasetStateRunning   = entity.DatasetStateRunning
	DatasetStateCompleted = entity.DatasetStateCompleted
)

// BacklogEntry is one scheduling unit appended to a KB's backlog (Option E
// §11.4). It carries the doc id plus the original event kind so the consumer
// can re-apply the same tombstone handling as the broker-based design without
// re-reading the queue. The backlog is an ordered log; the last event for a
// given doc (by append order) wins, so no sequence number is needed.
//
// Variants records the compile types (tree/structure/wiki/mindmap) produced by
// the doc-level compile for this doc, so the consumer can group a claimed batch
// into per-variant sub-batches and dispatch each to its own dataset-level
// compile path. It is empty (nil) for deleted events and for events published
// before this field existed; the consumer treats a nil/empty Variants as
// "unknown/legacy" and falls back to the legacy unified path.
type BacklogEntry struct {
	DocID     string   `json:"doc_id"`
	EventType string   `json:"event_type"`
	Variants  []string `json:"variants,omitempty"`
}

// ClaimResult is returned by Scheduler.Claim: the KB's tenant plus the closed
// batch of entries moved into inflight, and the token identifying this claim.
type ClaimResult struct {
	DatasetID string
	TenantID  string
	Entries   []BacklogEntry
	Token     string
}

// Publisher is the producer-side role (Option E §11.4). It appends a document
// event to the dataset's durable backlog and wakes idle workers. MySQL is the
// system of record; NATS notify is only a best-effort wake-up. Publish is the
// single producer entry point so a publisher never forgets to Notify after
// enqueue (the two are now one atomic call site).
type Publisher interface {
	// Provision ensures the backing store (table + notify subject) exists.
	Provision(ctx context.Context) error

	// Publish records one doc event in the dataset's durable backlog and wakes
	// idle workers. It is transactional so concurrent publishers do not clobber
	// each other's backlog, and it always pairs the append with a notify.
	// variants carries the doc-level compile types (tree/structure/wiki/mindmap)
	// for completed events; it is nil for deleted events.
	Publish(ctx context.Context, tenantID, datasetID, docID, eventType string, variants []string) error
}

// Claimer is the consumer-side role (Option E §11.5). A cluster of competing
// workers claims closed batches (backlog -> inflight, a move not a copy) per
// dataset, keeps the lease alive while processing, and acks when done. The claim
// is per-KB, so the same dataset is serialized by its single live lease.
type Claimer interface {
	// TryClaim finds a dataset with ready backlog and no live lease, or reclaims
	// an expired lease, claims it atomically, and returns the closed batch.
	// ok is false when there is nothing to claim or reclaim.
	TryClaim(ctx context.Context) (ClaimResult, bool, error)
	// Claim claims datasetID directly (no batch-size argument; the implementation
	// decides the batch boundary). acquired=false when a live lease already holds
	// the row (the race was lost) or the backlog is empty.
	Claim(ctx context.Context, datasetID string) (ClaimResult, bool, error)
	// TouchClaim extends the lease while processing (heartbeat). Returns false
	// when the lease is gone (taken over / reclaimed) so the worker must abort.
	TouchClaim(ctx context.Context, datasetID, token string, ttl time.Duration) (bool, error)
	// Ack removes the claimed batch from inflight; clears the claim metadata
	// only when backlog is also empty.
	Ack(ctx context.Context, datasetID, token string, batch []BacklogEntry) (backlogRemaining int, err error)

	// SetError records a diagnostic message for a failed batch without changing
	// the lifecycle state (the batch is left for retry, so state stays running).
	// It is best-effort for observability. token must be the claim token of the
	// batch that failed: the update only applies while that exact claim is still
	// live, so a stale worker whose lease was reclaimed cannot overwrite the
	// status of the worker that took over.
	SetError(ctx context.Context, datasetID, token, errMsg string) error
	// UpdateProgress records the current batch phase and progress while the
	// claim is live. Updates from a stale worker are ignored by token.
	UpdateProgress(ctx context.Context, datasetID, token string, progress float64, phase, message string) error

	// CancelInflight drops the live claim for a dataset back into the backlog so
	// a rewrite can start from a clean index. The token is intentionally NOT
	// checked: this is the rewrite's authoritative cancel. Workers verify their
	// claim token inside the dataset write lock before every destructive write, so
	// a cancelled worker cannot write after the rebuild clears storage. It is a safe no-op when
	// there is no live lease (the dataset is idle or already drained). The dropped
	// batch is re-marked pending (not deleted) so it is reclaimed and re-checked
	CancelInflight(ctx context.Context, datasetID, token string) error

	// WithDatasetLock executes fn while holding an exclusive per-dataset lock
	// that serializes against both writer side effects and dataset rebuilds
	// (RebuildDataset). It is the synchronization that closes the TOCTOU gap
	// between a worker's claim validation and its destructive write: the caller
	// must compare its claim token and perform the write inside fn. fn
	// runs inside a transaction that holds the dataset row lock for its whole
	// duration (MySQL) or a per-row mutex (Fake), so rebuild and writes are
	// mutually exclusive. claimToken is read under the lock; the caller must not
	// re-read it on a separate connection.
	WithDatasetLock(ctx context.Context, datasetID string, fn func(claimToken string) error) error

	// SubscribeNotify returns a channel of dataset ids pushed by Publish, or nil
	// when the implementation has no push wake-up (callers fall back to polling).
	SubscribeNotify(ctx context.Context) (<-chan string, error)
}

// newScheduler is the production constructor: a MySQL-backed scheduler with an
// optional NATS wake-up. holder identifies this ingestor instance; ttl is the
// claim lease duration. The returned *mysqlScheduler satisfies both Publisher
// and Claimer.
func newScheduler(db *gorm.DB, mq engine.MessageQueue, holder string, ttl time.Duration) *mysqlScheduler {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &mysqlScheduler{db: db, mq: mq, holder: holder, leaseTTL: ttl, claimBatchSize: defaultClaimBatch}
}

// ---- JSON helpers (the *_doc_ids columns are TEXT holding []BacklogEntry) ----

func marshalEntries(es []BacklogEntry) string {
	if len(es) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(es)
	return string(b)
}

func parseEntries(s string) []BacklogEntry {
	if s == "" || s == "[]" {
		return nil
	}
	var es []BacklogEntry
	if err := json.Unmarshal([]byte(s), &es); err != nil || len(es) == 0 {
		return nil
	}
	return es
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---- MySQL-backed scheduler ----

type mysqlScheduler struct {
	db             *gorm.DB
	mq             engine.MessageQueue
	holder         string
	leaseTTL       time.Duration
	claimBatchSize int
}

// defaultClaimBatch is the closed-batch boundary when Claim is invoked without
// an explicit size (the previous batchSize argument).
const defaultClaimBatch = 32

func (s *mysqlScheduler) Provision(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx).AutoMigrate(&entity.KnowledgeCompileDataset{})
}

// Publish appends one doc event to the dataset's durable backlog and wakes idle
// workers. The append is transactional (concurrent publishers do not clobber
// each other's backlog), and the notify is always paired with the append so a
// producer never needs a separate Notify call.
func (s *mysqlScheduler) Publish(ctx context.Context, tenantID, datasetID, docID, eventType string, variants []string) error {
	return s.publishEntry(ctx, tenantID, datasetID, BacklogEntry{DocID: docID, EventType: eventType, Variants: variants})
}

// loadRow fetches the dataset scheduling row (no lock). Returns nil (no error)
// when the row does not exist yet.
func (s *mysqlScheduler) loadRow(ctx context.Context, datasetID string) (*entity.KnowledgeCompileDataset, error) {
	var row entity.KnowledgeCompileDataset
	if err := s.db.WithContext(ctx).Where("dataset_id = ?", datasetID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (s *mysqlScheduler) publish(ctx context.Context, tenantID, datasetID, docID, eventType string, variants []string) error {
	return s.publishEntry(ctx, tenantID, datasetID, BacklogEntry{DocID: docID, EventType: eventType, Variants: variants})
}

func (s *mysqlScheduler) publishEntry(ctx context.Context, tenantID, datasetID string, entry BacklogEntry) error {
	if s.db == nil {
		return nil
	}
	// KnowledgeCompileDataset.dataset_id is the PRIMARY KEY, so the SELECT ... FOR
	// UPDATE below also takes an InnoDB gap lock for a not-yet-existing dataset;
	// concurrent publishers for the same dataset serialize on that lock and the
	// loser observes the row already present. FirstOrCreate then reliably finds
	// or inserts exactly one row, and the append below runs under the same row
	// lock so concurrent appends from different publishers cannot interleave.
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Set the key/tenant fields on the struct itself (not only via a string
		// Where) so the inserted row is fully populated and later queries by
		// dataset_id find it (M19: a raw-string Where alone leaves DatasetID
		// blank on the created row).
		row := entity.KnowledgeCompileDataset{
			DatasetID:      datasetID,
			TenantID:       tenantID,
			BacklogDocIDs:  "[]",
			InflightDocIDs: "[]",
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("dataset_id = ?", datasetID).
			FirstOrCreate(&row).Error; err != nil {
			return err
		}
		if row.TenantID == "" {
			row.TenantID = tenantID
		}
		backlog := parseEntries(row.BacklogDocIDs)
		backlog = append(backlog, entry)
		row.BacklogDocIDs = marshalEntries(backlog)
		// Surface a pending state unless a worker is already running (a live lease
		// means the consumer is mid-merge on this KB; a pending transition would
		// wrongly hide that). completed -> pending when new work arrives.
		if row.State != DatasetStateRunning {
			row.State = DatasetStatePending
		}
		return tx.Save(&row).Error
	})
	if err != nil {
		return fmt.Errorf("knowledge_compile: publish backlog %s: %w", datasetID, err)
	}
	common.Info("knowledge_compile: published backlog entry",
		zap.String("dataset_id", datasetID),
		zap.String("tenant_id", tenantID),
		zap.String("doc_id", entry.DocID),
		zap.String("event_type", entry.EventType))
	return s.notify(ctx, datasetID)
}

// WithDatasetLock executes fn under an exclusive per-dataset row lock. It locks
// the dataset row (SELECT ... FOR UPDATE) and runs fn inside the same
// transaction, so the row lock is held for the entire duration of fn and any
// concurrent WithDatasetLock on the same dataset blocks until fn returns. This
// is what makes a worker's claim-token check + destructive write atomic against
// a concurrent RebuildDataset.
//
// The claim token passed to fn is read within this transaction, so the check in
// fn does not need (and must not attempt) a separate DB read that would deadlock
// against the held row lock. fn runs inside the transaction; if it returns an
// error the transaction is rolled back
// and the error is propagated unchanged (any DB write performed inside fn is
// rolled back, so no partial side effect persists on an aborted write).
func (s *mysqlScheduler) WithDatasetLock(ctx context.Context, datasetID string, fn func(claimToken string) error) error {
	if s.db == nil {
		// No DB (test/no-op scheduler): nothing to serialize against.
		return fn("")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := entity.KnowledgeCompileDataset{
			DatasetID:      datasetID,
			BacklogDocIDs:  "[]",
			InflightDocIDs: "[]",
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("dataset_id = ?", datasetID).
			FirstOrCreate(&row).Error; err != nil {
			return err
		}
		return fn(row.ClaimToken)
	})
}

// claimRow atomically claims the closed batch from the row identified by
// datasetID: it takes a FOR UPDATE row lock, refuses a live lease, moves up to
// claimBatchSize entries from backlog to inflight, and stamps the lease. The
// callers (Claim and TryClaim) differ only in how they pick the datasetID; the
// claim itself is identical so both share this helper and stay race-free.
func (s *mysqlScheduler) claimRow(ctx context.Context, tx *gorm.DB, datasetID string) (ClaimResult, bool, error) {
	var row entity.KnowledgeCompileDataset
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("dataset_id = ?", datasetID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ClaimResult{}, false, nil // nothing to claim
		}
		return ClaimResult{}, false, err
	}
	now := time.Now()
	liveLease := row.ClaimOwner != "" && row.ClaimExpiresAt != nil && row.ClaimExpiresAt.After(now)
	if liveLease {
		return ClaimResult{}, false, nil // a live lease means some worker is already processing this KB
	}
	backlog := parseEntries(row.BacklogDocIDs)
	if len(backlog) == 0 {
		return ClaimResult{}, false, nil
	}
	n := minInt(s.claimBatchSize, len(backlog))
	batch := backlog[:n]
	inflight := parseEntries(row.InflightDocIDs)
	inflight = append(inflight, batch...)
	row.BacklogDocIDs = marshalEntries(backlog[n:])
	row.InflightDocIDs = marshalEntries(inflight)
	row.ClaimOwner = s.holder
	row.ClaimToken = generateHolder()
	exp := now.Add(s.leaseTTL)
	row.ClaimExpiresAt = &exp
	row.State = DatasetStateRunning
	row.ErrorMsg = ""
	row.Progress = 0
	row.CurrentPhase = "claiming"
	row.ProgressMsg = ""
	if err := tx.Save(&row).Error; err != nil {
		return ClaimResult{}, false, err
	}
	common.Info("knowledge_compile: claimed dataset batch",
		zap.String("dataset_id", row.DatasetID),
		zap.String("tenant_id", row.TenantID),
		zap.Int("batch_size", len(batch)),
		zap.Int("backlog_remaining", len(backlog)-n),
		zap.String("token", row.ClaimToken),
		zap.Any("batch", batch))
	return ClaimResult{DatasetID: row.DatasetID, TenantID: row.TenantID, Entries: batch, Token: row.ClaimToken}, true, nil
}

func (s *mysqlScheduler) Claim(ctx context.Context, datasetID string) (ClaimResult, bool, error) {
	if s.db == nil {
		return ClaimResult{}, false, nil
	}
	var res ClaimResult
	var acquired bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cr, ok, e := s.claimRow(ctx, tx, datasetID)
		if e != nil {
			return e
		}
		res, acquired = cr, ok
		return nil
	})
	if err != nil {
		return ClaimResult{}, false, err
	}
	return res, acquired, nil
}

func (s *mysqlScheduler) Ack(ctx context.Context, datasetID, token string, batch []BacklogEntry) (int, error) {
	if s.db == nil {
		return 0, nil
	}
	var remaining int
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row entity.KnowledgeCompileDataset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("dataset_id = ?", datasetID).First(&row).Error; err != nil {
			return err
		}
		if row.ClaimToken != token {
			return ErrClaimTokenMismatch
		}
		inflight := parseEntries(row.InflightDocIDs)
		inflight = removeEntries(inflight, batch)
		row.InflightDocIDs = marshalEntries(inflight)
		if len(inflight) == 0 {
			row.ClaimOwner = ""
			row.ClaimToken = ""
			row.ClaimExpiresAt = nil
		}
		remaining = len(parseEntries(row.BacklogDocIDs))
		// Backlog still has work -> pending; drained to empty -> completed.
		if remaining > 0 {
			row.State = DatasetStatePending
		} else {
			row.State = DatasetStateCompleted
			now := time.Now()
			row.LastCompletedAt = &now
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	common.Info("knowledge_compile: acked dataset batch",
		zap.String("dataset_id", datasetID),
		zap.Int("batch_size", len(batch)),
		zap.Int("backlog_remaining", remaining),
		zap.Any("batch", batch))
	return remaining, nil
}

// SetError records a best-effort diagnostic on the dataset row when a batch
// merge fails (consumer failure path). It does not change the lifecycle state:
// a failed batch is left in backlog for retry, so the row stays running/pending
// and the error message is surfaced by the status API for diagnosis.
//
// token scopes the write to the exact claim that failed: the update only lands
// while that claim token is still live on the row. If the original worker's
// lease expired and another worker took over (a new claim token), a stale
// SetError from the old worker is a no-op and cannot overwrite the new worker's
// status/diagnostic.
func (s *mysqlScheduler) SetError(ctx context.Context, datasetID, token, errMsg string) error {
	if s.db == nil {
		return nil
	}
	msg := errMsg
	if len(msg) > 4000 {
		msg = msg[:4000]
	}
	res := s.db.WithContext(ctx).Model(&entity.KnowledgeCompileDataset{}).
		Where("dataset_id = ? AND claim_token = ?", datasetID, token).
		Update("error_msg", msg)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		common.Warn("knowledge_compile: set_error ignored (claim token mismatch / lease reclaimed)",
			zap.String("dataset_id", datasetID))
	}
	return nil
}

const progressMessageMaxLength = 3000

func timestampProgressMessage(message string) string {
	if message == "" {
		return ""
	}
	return time.Now().Format("15:04:05") + " " + message
}

func appendProgressMessage(previous, message string) string {
	if message == "" {
		return previous
	}
	if previous == "" {
		if len([]rune(message)) > progressMessageMaxLength {
			return string([]rune(message)[len([]rune(message))-progressMessageMaxLength:])
		}
		return message
	}
	joined := previous + "\n" + message
	if len([]rune(joined)) > progressMessageMaxLength {
		runes := []rune(joined)
		return string(runes[len(runes)-progressMessageMaxLength:])
	}
	return joined
}

// UpdateProgress records a best-effort phase update for the active claim. The
// token check prevents a worker whose lease was reclaimed from overwriting the
// progress of the new owner.
func (s *mysqlScheduler) UpdateProgress(ctx context.Context, datasetID, token string, progress float64, phase, message string) error {
	if s.db == nil {
		return nil
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row entity.KnowledgeCompileDataset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("dataset_id = ?", datasetID).First(&row).Error; err != nil {
			return err
		}
		if row.ClaimToken != token {
			return nil
		}
		row.Progress = progress
		row.CurrentPhase = phase
		row.ProgressMsg = appendProgressMessage(row.ProgressMsg, message)
		return tx.Save(&row).Error
	})
}

// CancelInflight drops the live claim for a dataset back into the backlog so a
// rewrite can start from a clean index. It is a no-op when there is no live
// lease. The dropped batch is re-marked pending and is reprocessed after the
// rebuild republishes the dataset documents.
func (s *mysqlScheduler) CancelInflight(ctx context.Context, datasetID, token string) error {
	if s.db == nil {
		return nil
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row entity.KnowledgeCompileDataset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("dataset_id = ?", datasetID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		// No live lease (idle or already drained) → nothing to cancel.
		now := time.Now()
		liveLease := row.ClaimOwner != "" && row.ClaimExpiresAt != nil && row.ClaimExpiresAt.After(now)
		if !liveLease {
			return nil
		}
		inflight := parseEntries(row.InflightDocIDs)
		backlog := parseEntries(row.BacklogDocIDs)
		backlog = append(backlog, inflight...)
		row.InflightDocIDs = "[]"
		row.ClaimOwner = ""
		row.ClaimToken = ""
		row.ClaimExpiresAt = nil
		row.BacklogDocIDs = marshalEntries(backlog)
		row.State = DatasetStatePending
		return tx.Save(&row).Error
	})
	if err != nil {
		return fmt.Errorf("knowledge_compile: cancel inflight %s: %w", datasetID, err)
	}
	return nil
}

func (s *mysqlScheduler) TouchClaim(ctx context.Context, datasetID, token string, ttl time.Duration) (bool, error) {
	if s.db == nil {
		return false, nil
	}
	res := s.db.WithContext(ctx).Model(&entity.KnowledgeCompileDataset{}).
		Where("dataset_id = ? AND claim_token = ?", datasetID, token).
		Update("claim_expires_at", time.Now().Add(ttl))
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		common.Warn("knowledge_compile: claim touch failed (lease lost or token mismatch)",
			zap.String("dataset_id", datasetID), zap.String("token", token))
	}
	return res.RowsAffected > 0, nil
}

// TryClaim finds a dataset with ready backlog and no live lease, or reclaims an
// expired lease, claims it atomically within a single transaction, and returns
// the closed batch. ok is false when there is nothing to claim or reclaim.
//
// The find and the claim run inside one locked transaction so there is no race
// window between picking a dataset and taking its lease: a concurrent worker
// cannot claim the same row out from under us. Reclaim is inlined here so the
// sweeper and the poller share one entry point (no separate ReclaimExpired).
func (s *mysqlScheduler) TryClaim(ctx context.Context) (ClaimResult, bool, error) {
	if s.db == nil {
		return ClaimResult{}, false, nil
	}
	var res ClaimResult
	var acquired bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		// 1) a ready dataset (backlog, no live lease) — claim it directly.
		if id, ok := s.findClaimableID(ctx, tx, now); ok {
			cr, ok, e := s.claimRow(ctx, tx, id)
			if e != nil {
				return e
			}
			res, acquired = cr, ok
			return nil
		}
		// 2) reclaim an expired lease back into backlog, then claim it.
		if id, ok, e := s.reclaimOne(ctx, tx, now); e != nil {
			return e
		} else if ok {
			cr, ok, e := s.claimRow(ctx, tx, id)
			if e != nil {
				return e
			}
			res, acquired = cr, ok
			return nil
		}
		return nil
	})
	if err != nil {
		return ClaimResult{}, false, err
	}
	return res, acquired, nil
}

// findClaimableID returns one dataset id with non-empty backlog and no live
// lease, or ok=false when none exists. It runs inside caller's transaction so
// the returned id is still locked when the caller claims it.
func (s *mysqlScheduler) findClaimableID(ctx context.Context, tx *gorm.DB, now time.Time) (string, bool) {
	var id string
	err := tx.Model(&entity.KnowledgeCompileDataset{}).
		Where("(claim_expires_at IS NULL OR claim_expires_at <= ?) AND backlog_doc_ids <> '[]' AND backlog_doc_ids <> '' AND backlog_doc_ids IS NOT NULL", now).
		Order("priority DESC, updated_at ASC").
		Limit(1).
		Pluck("dataset_id", &id).Error
	if err != nil || id == "" {
		return "", false
	}
	return id, true
}

// reclaimOne moves one expired inflight batch back to backlog (crash recovery)
// and returns that dataset id, or ok=false when nothing is expired. It runs
// inside caller's transaction so the reclaimed row remains locked when the
// caller claims it.
func (s *mysqlScheduler) reclaimOne(ctx context.Context, tx *gorm.DB, now time.Time) (string, bool, error) {
	var rows []entity.KnowledgeCompileDataset
	if err := tx.Model(&entity.KnowledgeCompileDataset{}).
		Where("claim_expires_at IS NOT NULL AND claim_expires_at <= ? AND inflight_doc_ids <> '[]' AND inflight_doc_ids <> ''", now).
		Find(&rows).Error; err != nil {
		return "", false, err
	}
	for _, row := range rows {
		var cur entity.KnowledgeCompileDataset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("dataset_id = ?", row.DatasetID).First(&cur).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return "", false, err
		}
		if cur.ClaimExpiresAt != nil && cur.ClaimExpiresAt.After(now) {
			continue
		}
		inflight := parseEntries(cur.InflightDocIDs)
		if len(inflight) == 0 {
			continue
		}
		backlog := parseEntries(cur.BacklogDocIDs)
		backlog = append(backlog, inflight...)
		cur.BacklogDocIDs = marshalEntries(backlog)
		cur.InflightDocIDs = "[]"
		cur.ClaimOwner = ""
		cur.ClaimToken = ""
		cur.ClaimExpiresAt = nil
		cur.State = DatasetStatePending
		if err := tx.Save(&cur).Error; err != nil {
			return "", false, err
		}
		common.Warn("knowledge_compile: reclaimed expired inflight lease",
			zap.String("dataset_id", cur.DatasetID),
			zap.String("tenant_id", cur.TenantID),
			zap.Int("reclaimed_entries", len(inflight)))
		return cur.DatasetID, true, nil
	}
	return "", false, nil
}

// notify is the best-effort wake-up published after a successful Publish. It is
// a no-op without a configured NATS subject; callers fall back to polling.
func (s *mysqlScheduler) notify(ctx context.Context, datasetID string) error {
	if s.mq == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{"dataset_id": datasetID})
	if err := s.mq.PublishKnowledgeCompile(notifySubject, payload); err != nil {
		common.Warn("knowledge_compile: publish notify failed (workers will poll)",
			zap.String("dataset_id", datasetID), zap.Error(err))
		return err
	}
	common.Info("knowledge_compile: published worker notify", zap.String("dataset_id", datasetID))
	return nil
}

func (s *mysqlScheduler) SubscribeNotify(ctx context.Context) (<-chan string, error) {
	if s.mq == nil {
		return nil, nil
	}
	return s.mq.SubscribeNotify(ctx)
}

// removeEntries drops every entry in batch from the inflight slice (match by
// doc_id + event_type). The batch is the exact set the worker claimed, so
// this is a precise removal, not a "clear all" — any inflight added by a
// concurrent claim is preserved.
//
// The key is doc_id+"\x00"+event_type: BacklogEntry now carries a Variants
// slice which is not comparable, so it cannot be used as a Go map key. Variants
// deliberately does not participate in the removal match — Ack still removes by
// doc_id + event_type so an entry whose Variants differ from the inflight copy
// is still removed exactly once.
func removeEntries(inflight, batch []BacklogEntry) []BacklogEntry {
	if len(batch) == 0 {
		return inflight
	}
	drop := make(map[string]bool, len(batch))
	for _, e := range batch {
		drop[e.DocID+"\x00"+e.EventType] = true
	}
	out := make([]BacklogEntry, 0, len(inflight))
	for _, e := range inflight {
		if drop[e.DocID+"\x00"+e.EventType] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ---- in-memory scheduler for tests ----

type fakeRow struct {
	tenant       string
	backlog      []BacklogEntry
	inflight     []BacklogEntry
	owner        string
	token        string
	expires      *time.Time
	state        string
	errorMsg     string
	progress     float64
	currentPhase string
	progressMsg  string
	// writeMu serializes per-dataset writer side effects and rebuilds, mirroring
	// the MySQL scheduler's WithDatasetLock row lock.
	writeMu sync.Mutex
}

// FakeScheduler is an in-memory Publisher + Claimer used by tests. It mirrors
// the MySQL semantics (move-not-copy claim, token-checked ack, lease takeover).
type FakeScheduler struct {
	mu       sync.Mutex
	rows     map[string]*fakeRow
	notifyCh chan string
	holder   string
	leaseTTL time.Duration
}

// NewFakeScheduler constructs an in-memory scheduler for tests.
func NewFakeScheduler() *FakeScheduler {
	return &FakeScheduler{
		rows:     map[string]*fakeRow{},
		notifyCh: make(chan string, 64),
		holder:   generateHolder(),
		leaseTTL: 2 * time.Minute,
	}
}

func (f *FakeScheduler) Provision(_ context.Context) error { return nil }

// Publish appends one doc event and pushes a notify (same contract as MySQL).
func (f *FakeScheduler) Publish(_ context.Context, tenantID, datasetID, docID, eventType string, variants []string) error {
	return f.publishEntry(tenantID, datasetID, BacklogEntry{DocID: docID, EventType: eventType, Variants: variants})
}

// PublishedCount returns the total number of backlog events published across
// all datasets. Tests use it to assert that a code path did not trigger a
// dataset-level knowledge-compile rebuild (PublishCompleted).
func (f *FakeScheduler) PublishedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.rows {
		n += len(r.backlog)
	}
	return n
}

func (f *FakeScheduler) publishEntry(tenantID, datasetID string, entry BacklogEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[datasetID]
	if !ok {
		r = &fakeRow{tenant: tenantID}
		f.rows[datasetID] = r
	}
	if r.tenant == "" {
		r.tenant = tenantID
	}
	r.backlog = append(r.backlog, entry)
	// Mirror the MySQL scheduler: surface pending unless a worker is running.
	if r.state != DatasetStateRunning {
		r.state = DatasetStatePending
	}
	select {
	case f.notifyCh <- datasetID:
	default:
	}
	return nil
}

func (f *FakeScheduler) Claim(_ context.Context, datasetID string) (ClaimResult, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[datasetID]
	if !ok {
		return ClaimResult{}, false, nil
	}
	now := time.Now()
	live := r.owner != "" && r.expires != nil && r.expires.After(now)
	if live {
		return ClaimResult{}, false, nil
	}
	if len(r.backlog) == 0 {
		return ClaimResult{}, false, nil
	}
	n := minInt(defaultClaimBatch, len(r.backlog))
	batch := append([]BacklogEntry{}, r.backlog[:n]...)
	r.inflight = append(r.inflight, batch...)
	r.backlog = append([]BacklogEntry{}, r.backlog[n:]...)
	r.owner = f.holder
	r.token = generateHolder()
	exp := now.Add(f.leaseTTL)
	r.expires = &exp
	r.state = DatasetStateRunning
	r.errorMsg = ""
	r.progress = 0
	r.currentPhase = "claiming"
	r.progressMsg = ""
	return ClaimResult{DatasetID: datasetID, TenantID: r.tenant, Entries: batch, Token: r.token}, true, nil
}

func (f *FakeScheduler) Ack(_ context.Context, datasetID, token string, batch []BacklogEntry) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[datasetID]
	if !ok {
		return 0, ErrClaimTokenMismatch
	}
	if r.token != token {
		return 0, ErrClaimTokenMismatch
	}
	r.inflight = removeEntries(r.inflight, batch)
	if len(r.inflight) == 0 {
		r.owner, r.token, r.expires = "", "", nil
	}
	// Mirror the MySQL scheduler: backlog still has work -> pending; drained -> completed.
	if len(r.backlog) > 0 {
		r.state = DatasetStatePending
	} else {
		r.state = DatasetStateCompleted
	}
	return len(r.backlog), nil
}

func (f *FakeScheduler) TouchClaim(_ context.Context, datasetID, token string, _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[datasetID]
	if !ok || r.token != token {
		return false, nil
	}
	exp := time.Now().Add(f.leaseTTL)
	r.expires = &exp
	return true, nil
}

func (f *FakeScheduler) SetError(_ context.Context, datasetID, token, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[datasetID]
	if !ok || r.token != token {
		return nil
	}
	r.errorMsg = errMsg
	return nil
}

func (f *FakeScheduler) UpdateProgress(_ context.Context, datasetID, token string, progress float64, phase, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[datasetID]
	if !ok || r.token != token {
		return nil
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	r.progress = progress
	r.currentPhase = phase
	r.progressMsg = appendProgressMessage(r.progressMsg, message)
	return nil
}

// WithDatasetLock executes fn under the per-row write mutex, mirroring the MySQL
// scheduler's row-lock semantics: a rebuild and a writer's destructive side
// effect on the same dataset are mutually exclusive. claimToken is read under
// the lock.
func (f *FakeScheduler) WithDatasetLock(_ context.Context, datasetID string, fn func(claimToken string) error) error {
	f.mu.Lock()
	r, ok := f.rows[datasetID]
	if !ok {
		r = &fakeRow{}
		f.rows[datasetID] = r
	}
	f.mu.Unlock()

	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	f.mu.Lock()
	claimToken := r.token
	f.mu.Unlock()
	return fn(claimToken)
}

// CancelInflight drops the live claim back into the backlog (mirrors MySQL).
func (f *FakeScheduler) CancelInflight(_ context.Context, datasetID, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[datasetID]
	if !ok {
		return nil
	}
	now := time.Now()
	live := r.owner != "" && r.expires != nil && r.expires.After(now)
	if !live {
		return nil
	}
	r.backlog = append(r.backlog, r.inflight...)
	r.inflight = nil
	r.owner, r.token, r.expires = "", "", nil
	r.state = DatasetStatePending
	return nil
}

// fakeReclaimExpired finds one dataset with an expired lease and non-empty
// inflight, moves the inflight batch back to backlog, clears the lease, and
// marks the row pending — mirroring the MySQL scheduler's reclaimOne. It returns
// the reclaimed dataset id, or "" when nothing is expired. The lock must already
// be held by the caller.
func (f *FakeScheduler) fakeReclaimExpired(now time.Time) string {
	for id, r := range f.rows {
		if r.owner != "" && r.expires != nil && r.expires.After(now) {
			continue
		}
		if len(r.inflight) > 0 {
			r.backlog = append(r.backlog, r.inflight...)
			r.inflight = nil
			r.owner, r.token, r.expires = "", "", nil
			// reclaimOne: inflight -> backlog, lease cleared -> pending.
			r.state = DatasetStatePending
			return id
		}
	}
	return ""
}

// TryClaim mirrors the production flow: claim a ready dataset, otherwise reclaim
// an expired lease and claim it.
func (f *FakeScheduler) TryClaim(ctx context.Context) (ClaimResult, bool, error) {
	f.mu.Lock()
	now := time.Now()
	var readyID, expiredID string
	for id, r := range f.rows {
		if len(r.backlog) > 0 && (r.owner == "" || r.expires == nil || !r.expires.After(now)) {
			readyID = id
			break
		}
	}
	if readyID == "" {
		expiredID = f.fakeReclaimExpired(now)
	}
	f.mu.Unlock()
	if readyID != "" {
		return f.Claim(ctx, readyID)
	}
	if expiredID != "" {
		return f.Claim(ctx, expiredID)
	}
	return ClaimResult{}, false, nil
}

func (f *FakeScheduler) SubscribeNotify(_ context.Context) (<-chan string, error) {
	return f.notifyCh, nil
}
