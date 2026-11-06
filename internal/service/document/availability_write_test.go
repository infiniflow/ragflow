package document

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

type zeroCountAvailabilityEngine struct {
	sourceAvailabilityDocEngine
	exists    bool
	existsErr error
}

// A stateful index fixture keeps successful batches when a later operation
// fails; a later database failure cannot undo those external writes.
type availabilityRecoveryEngine struct {
	fakeChatDocEngine
	rows         map[string]int
	updates      int
	failUpdateAt int
}

func (e *availabilityRecoveryEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return true, nil
}

func (e *availabilityRecoveryEngine) Search(_ context.Context, req *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	ids := make([]string, 0, len(e.rows))
	for id := range e.rows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	rows := make([]map[string]any, 0)
	for i := req.Offset; i < min(req.Offset+req.Limit, len(ids)); i++ {
		rows = append(rows, map[string]any{"id": ids[i], "available_int": e.rows[ids[i]]})
	}
	return &enginetypes.SearchResult{Chunks: rows, Total: int64(len(ids))}, nil
}

func (e *availabilityRecoveryEngine) UpdateChunks(_ context.Context, condition, values map[string]any, _, _ string) error {
	e.updates++
	if e.updates == e.failUpdateAt {
		return errors.New("injected index batch failure")
	}
	for _, id := range condition["id"].([]string) {
		e.rows[id] = values["available_int"].(int)
	}
	return nil
}

func setAvailabilityThroughEntry(ctx context.Context, svc *DocumentService, entry, status string) error {
	if entry == "batch" {
		_, _, err := svc.BatchUpdateDocumentStatus(ctx, "kb-1", status, []string{"doc-1"})
		return err
	}
	enabled, _ := strconv.Atoi(status)
	_, _, err := svc.UpdateDatasetDocument(ctx, "kb-1", "doc-1", &UpdateDatasetDocumentRequest{Enabled: &enabled}, map[string]bool{"enabled": true})
	return err
}

func TestDocumentAvailabilitySameStatusRepairsIndex(t *testing.T) {
	for _, entry := range []string{"single", "batch"} {
		for _, status := range []string{"0", "1"} {
			t.Run(entry+"/"+status, func(t *testing.T) {
				db := setupServiceTestDB(t)
				pushServiceDB(t, db)
				insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
				insertTestDoc(t, "doc-1", "kb-1", 0, 0)
				if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("status", status).Error; err != nil {
					t.Fatal(err)
				}
				want, _ := strconv.Atoi(status)
				eng := &availabilityRecoveryEngine{rows: map[string]int{"source": 1 - want}}
				svc := testDocumentService(t)
				svc.docEngine = eng
				if err := setAvailabilityThroughEntry(documentTestContext(t.Context()), svc, entry, status); err != nil {
					t.Fatal(err)
				}
				persisted, err := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
				if err != nil {
					t.Fatal(err)
				}
				if persisted.Status == nil || *persisted.Status != status {
					t.Fatalf("same-status repair changed database status: %v, want %s", persisted.Status, status)
				}
				if eng.rows["source"] != want {
					t.Fatalf("successful same-status request left index availability=%d, want %d", eng.rows["source"], want)
				}
			})
		}
	}
}

func TestDocumentAvailabilityRecoversAfterPartialFailure(t *testing.T) {
	for _, failure := range []string{"index_batch", "database_update"} {
		for _, previous := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/previous=%d", failure, previous), func(t *testing.T) {
				db := setupServiceTestDB(t)
				pushServiceDB(t, db)
				insertTestKB(t, "kb-1", "tenant-1", 1, 0, 1001)
				insertTestDoc(t, "doc-1", "kb-1", 0, 1001)
				if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("status", strconv.Itoa(previous)).Error; err != nil {
					t.Fatal(err)
				}
				eng := &availabilityRecoveryEngine{rows: make(map[string]int)}
				for i := range 1001 {
					eng.rows[fmt.Sprintf("chunk-%04d", i)] = previous
				}
				svc := testDocumentService(t)
				svc.docEngine = eng
				if failure == "index_batch" {
					eng.failUpdateAt = 2
				}
				if failure == "database_update" {
					if err := db.Callback().Update().Before("gorm:update").Register("test:failed_status_update", func(tx *gorm.DB) {
						tx.AddError(errors.New("injected database update failure"))
					}); err != nil {
						t.Fatal(err)
					}
				}
				ctx := documentTestContext(t.Context())
				if err := setAvailabilityThroughEntry(ctx, svc, "batch", strconv.Itoa(1-previous)); err == nil {
					t.Fatal("injected failure was acknowledged as success")
				}
				var persisted entity.Document
				if err := db.Where("id = ?", "doc-1").Take(&persisted).Error; err != nil {
					t.Fatal(err)
				}
				if *persisted.Status != strconv.Itoa(previous) || eng.rows["chunk-0000"] == previous {
					t.Fatalf("expected unchanged database status with a changed index batch: status=%s index=%d", *persisted.Status, eng.rows["chunk-0000"])
				}
				eng.failUpdateAt = 0
				if failure == "database_update" {
					db.Callback().Update().Remove("test:failed_status_update")
				}
				// Explicitly restoring the DB's old status must repair every row,
				// including a batch that succeeded before the failure.
				if err := setAvailabilityThroughEntry(ctx, svc, "batch", strconv.Itoa(previous)); err != nil {
					t.Fatal(err)
				}
				for id, available := range eng.rows {
					if available != previous {
						t.Fatalf("%s remains available=%d after successful recovery to %d", id, available, previous)
					}
				}
			})
		}
	}
}

func (e *zeroCountAvailabilityEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return e.exists, e.existsErr
}

// Index rows can be visible before the ingestor applies its source counters.
func TestBatchAvailabilityDoesNotTrustPendingChunkCounts(t *testing.T) {
	for _, mode := range []string{"indexed_before_counts", "empty_index", "index_check_failed"} {
		t.Run(mode, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
			insertTestDoc(t, "doc-1", "kb-1", 0, 0)
			if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("status", "1").Error; err != nil {
				t.Fatal(err)
			}
			eng := &zeroCountAvailabilityEngine{exists: mode == "indexed_before_counts"}
			if mode == "index_check_failed" {
				eng.existsErr = errors.New("index unavailable")
			}
			svc := testDocumentService(t)
			svc.docEngine = eng
			_, code, err := svc.BatchUpdateDocumentStatus(documentTestContext(t.Context()), "kb-1", "0", []string{"doc-1"})
			doc, loadErr := dao.NewDocumentDAO().GetByID(t.Context(), db, "doc-1")
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if mode == "index_check_failed" {
				if err == nil || *doc.Status != "1" {
					t.Fatalf("failed storage check acknowledged or changed status: code=%d err=%v status=%v", code, err, *doc.Status)
				}
				return
			}
			if err != nil || code != common.CodeSuccess || *doc.Status != "0" {
				t.Fatalf("disable failed: code=%d err=%v status=%v", code, err, *doc.Status)
			}
			wantUpdates := 0
			if eng.exists {
				wantUpdates = 1
			}
			if len(eng.updateConditions) != wantUpdates {
				t.Fatalf("visibility updates=%d, want %d regardless of pending zero counters", len(eng.updateConditions), wantUpdates)
			}
		})
	}
}
