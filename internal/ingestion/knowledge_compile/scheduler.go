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

	"ragflow/internal/engine"
	"ragflow/internal/entity"

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

// BacklogEntry is one scheduling unit appended to a KB's backlog (Option E
// §11.4). It carries the doc id plus the original event kind/seq so the
// consumer can re-apply the same out-of-order / tombstone guards as the
// broker-based design without re-reading the queue.
type BacklogEntry struct {
	DocID     string `json:"doc_id"`
	EventType string `json:"event_type"`
	Seq       uint64 `json:"seq"`
}

// ClaimResult is returned by Scheduler.Claim: the KB's tenant plus the closed
// batch of entries moved into inflight, and the token identifying this claim.
type ClaimResult struct {
	TenantID string
	Entries  []BacklogEntry
	Token    string
}

// Scheduler owns the claim/ack lifecycle against the durable scheduling store
// (Option E §11.4/§11.5). MySQL is the system of record; NATS notify is only a
// wake-up. The claim is a move (backlog -> inflight), never a copy, so a doc id
// is visible to at most one worker at a time; same-KB serialization follows
// from the inflight set, not from the broker.
type Scheduler interface {
	// Provision ensures the backing store (table + notify subject) exists.
	Provision(ctx context.Context) error
	// AppendBacklog adds one doc event to the KB's backlog. The append is
	// transactional and preserves any existing backlog (concurrent publishers
	// do not clobber each other).
	AppendBacklog(ctx context.Context, tenantID, datasetID, docID, eventType string, seq uint64) error
	// Claim moves a bounded prefix of backlog -> inflight for datasetID and
	// returns the closed batch. acquired=false when the row is held by another
	// live lease (the race was lost) or the backlog is empty.
	Claim(ctx context.Context, datasetID string, batchSize int) (ClaimResult, bool, error)
	// Ack removes the claimed batch from inflight; clears the claim metadata
	// only when backlog is also empty. Returns the remaining backlog size.
	Ack(ctx context.Context, datasetID, token string, batch []BacklogEntry) (backlogRemaining int, err error)
	// TouchClaim extends the lease while processing (heartbeat). Returns false
	// when the lease is gone (taken over / reclaimed) so the worker must abort.
	TouchClaim(ctx context.Context, datasetID, token string, ttl time.Duration) (bool, error)
	// FindClaimable returns up to limit dataset ids with non-empty backlog and
	// no live lease (a free token to claim).
	FindClaimable(ctx context.Context, limit int) ([]string, error)
	// Notify wakes idle workers about a dataset that just got backlog.
	Notify(ctx context.Context, datasetID string) error
	// SubscribeNotify returns a channel of dataset ids pushed by Notify, or nil
	// when the implementation has no push wake-up (callers fall back to polling).
	SubscribeNotify(ctx context.Context) (<-chan string, error)
	// ReclaimExpired moves expired inflight entries back to backlog (sweeper).
	ReclaimExpired(ctx context.Context, now time.Time) (int, error)
}

// newScheduler is the production constructor: a MySQL-backed scheduler with an
// optional NATS wake-up. holder identifies this ingestor instance; ttl is the
// claim lease duration.
func newScheduler(db *gorm.DB, mq engine.MessageQueue, holder string, ttl time.Duration) Scheduler {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &mysqlScheduler{db: db, mq: mq, holder: holder, leaseTTL: ttl}
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
	db       *gorm.DB
	mq       engine.MessageQueue
	holder   string
	leaseTTL time.Duration
}

func (s *mysqlScheduler) Provision(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx).AutoMigrate(&entity.KnowledgeCompileDoc{})
}

func (s *mysqlScheduler) AppendBacklog(ctx context.Context, tenantID, datasetID, docID, eventType string, seq uint64) error {
	if s.db == nil {
		return nil
	}
	entry := BacklogEntry{DocID: docID, EventType: eventType, Seq: seq}
	// KnowledgeCompileDoc.dataset_id is the PRIMARY KEY, so the SELECT ... FOR
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
		row := entity.KnowledgeCompileDoc{
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
		return tx.Save(&row).Error
	})
	if err != nil {
		return fmt.Errorf("knowledge_compile: append backlog %s: %w", datasetID, err)
	}
	return nil
}

func (s *mysqlScheduler) Claim(ctx context.Context, datasetID string, batchSize int) (ClaimResult, bool, error) {
	if s.db == nil {
		return ClaimResult{}, false, nil
	}
	var res ClaimResult
	var acquired bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row entity.KnowledgeCompileDoc
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("dataset_id = ?", datasetID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil // nothing to claim
			}
			return err
		}
		now := time.Now()
		liveLease := row.ClaimOwner != "" && row.ClaimExpiresAt != nil && row.ClaimExpiresAt.After(now)
		if liveLease {
			return nil // a live lease means some worker is already processing this KB
		}
		backlog := parseEntries(row.BacklogDocIDs)
		if len(backlog) == 0 {
			return nil
		}
		n := minInt(batchSize, len(backlog))
		batch := backlog[:n]
		inflight := parseEntries(row.InflightDocIDs)
		inflight = append(inflight, batch...)
		row.BacklogDocIDs = marshalEntries(backlog[n:])
		row.InflightDocIDs = marshalEntries(inflight)
		row.ClaimOwner = s.holder
		row.ClaimToken = generateHolder()
		exp := now.Add(s.leaseTTL)
		row.ClaimExpiresAt = &exp
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		res = ClaimResult{TenantID: row.TenantID, Entries: batch, Token: row.ClaimToken}
		acquired = true
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
		var row entity.KnowledgeCompileDoc
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
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		remaining = len(parseEntries(row.BacklogDocIDs))
		return nil
	})
	if err != nil {
		return 0, err
	}
	return remaining, nil
}

func (s *mysqlScheduler) TouchClaim(ctx context.Context, datasetID, token string, ttl time.Duration) (bool, error) {
	if s.db == nil {
		return false, nil
	}
	res := s.db.WithContext(ctx).Model(&entity.KnowledgeCompileDoc{}).
		Where("dataset_id = ? AND claim_token = ?", datasetID, token).
		Update("claim_expires_at", time.Now().Add(ttl))
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (s *mysqlScheduler) FindClaimable(ctx context.Context, limit int) ([]string, error) {
	if s.db == nil || limit <= 0 {
		return nil, nil
	}
	var ids []string
	err := s.db.WithContext(ctx).Model(&entity.KnowledgeCompileDoc{}).
		Where("(claim_expires_at IS NULL OR claim_expires_at <= ?) AND backlog_doc_ids <> '[]' AND backlog_doc_ids <> '' AND backlog_doc_ids IS NOT NULL", time.Now()).
		Order("priority DESC, updated_at ASC").
		Limit(limit).
		Pluck("dataset_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *mysqlScheduler) Notify(ctx context.Context, datasetID string) error {
	if s.mq == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{"dataset_id": datasetID})
	return s.mq.PublishKnowledgeCompile(notifySubject, payload)
}

func (s *mysqlScheduler) SubscribeNotify(ctx context.Context) (<-chan string, error) {
	if s.mq == nil {
		return nil, nil
	}
	return s.mq.SubscribeNotify(ctx)
}

func (s *mysqlScheduler) ReclaimExpired(ctx context.Context, now time.Time) (int, error) {
	if s.db == nil {
		return 0, nil
	}
	var rows []entity.KnowledgeCompileDoc
	if err := s.db.WithContext(ctx).Model(&entity.KnowledgeCompileDoc{}).
		Where("claim_expires_at IS NOT NULL AND claim_expires_at <= ? AND inflight_doc_ids <> '[]' AND inflight_doc_ids <> ''", now).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	total := 0
	for _, row := range rows {
		inflight := parseEntries(row.InflightDocIDs)
		if len(inflight) == 0 {
			continue
		}
		total += len(inflight)
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var cur entity.KnowledgeCompileDoc
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("dataset_id = ?", row.DatasetID).First(&cur).Error; err != nil {
				return err
			}
			// Re-check liveness under the lock so we don't clobber a lease that
			// was refreshed between the scan and now.
			if cur.ClaimExpiresAt != nil && cur.ClaimExpiresAt.After(now) {
				return nil
			}
			backlog := parseEntries(cur.BacklogDocIDs)
			backlog = append(backlog, parseEntries(cur.InflightDocIDs)...)
			cur.BacklogDocIDs = marshalEntries(backlog)
			cur.InflightDocIDs = "[]"
			cur.ClaimOwner = ""
			cur.ClaimToken = ""
			cur.ClaimExpiresAt = nil
			return tx.Save(&cur).Error
		})
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// removeEntries drops every entry in batch from the inflight slice (match by
// doc_id + event_type + seq). The batch is the exact set the worker claimed, so
// this is a precise removal, not a "clear all" — any inflight added by a
// concurrent claim is preserved.
func removeEntries(inflight, batch []BacklogEntry) []BacklogEntry {
	if len(batch) == 0 {
		return inflight
	}
	drop := make(map[BacklogEntry]bool, len(batch))
	for _, e := range batch {
		drop[e] = true
	}
	out := make([]BacklogEntry, 0, len(inflight))
	for _, e := range inflight {
		if drop[e] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ---- in-memory scheduler for tests ----

type fakeRow struct {
	tenant   string
	backlog  []BacklogEntry
	inflight []BacklogEntry
	owner    string
	token    string
	expires  *time.Time
}

// FakeScheduler is an in-memory Scheduler used by tests. It mirrors the
// MySQL semantics (move-not-copy claim, token-checked ack, lease takeover).
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

func (f *FakeScheduler) AppendBacklog(_ context.Context, tenantID, datasetID, docID, eventType string, seq uint64) error {
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
	r.backlog = append(r.backlog, BacklogEntry{DocID: docID, EventType: eventType, Seq: seq})
	return nil
}

func (f *FakeScheduler) Claim(_ context.Context, datasetID string, batchSize int) (ClaimResult, bool, error) {
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
	n := minInt(batchSize, len(r.backlog))
	batch := append([]BacklogEntry{}, r.backlog[:n]...)
	r.inflight = append(r.inflight, batch...)
	r.backlog = append([]BacklogEntry{}, r.backlog[n:]...)
	r.owner = f.holder
	r.token = generateHolder()
	exp := now.Add(f.leaseTTL)
	r.expires = &exp
	return ClaimResult{TenantID: r.tenant, Entries: batch, Token: r.token}, true, nil
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

func (f *FakeScheduler) FindClaimable(_ context.Context, limit int) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	now := time.Now()
	for id, r := range f.rows {
		if len(r.backlog) == 0 {
			continue
		}
		if r.owner != "" && r.expires != nil && r.expires.After(now) {
			continue
		}
		ids = append(ids, id)
		if limit > 0 && len(ids) >= limit {
			break
		}
	}
	return ids, nil
}

func (f *FakeScheduler) Notify(_ context.Context, datasetID string) error {
	select {
	case f.notifyCh <- datasetID:
	default:
	}
	return nil
}

func (f *FakeScheduler) SubscribeNotify(_ context.Context) (<-chan string, error) {
	return f.notifyCh, nil
}

func (f *FakeScheduler) ReclaimExpired(_ context.Context, now time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, r := range f.rows {
		if r.owner != "" && r.expires != nil && r.expires.After(now) {
			continue
		}
		if len(r.inflight) == 0 {
			continue
		}
		total += len(r.inflight)
		r.backlog = append(r.backlog, r.inflight...)
		r.inflight = nil
		r.owner, r.token, r.expires = "", "", nil
	}
	return total, nil
}
