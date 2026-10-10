package document

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	"ragflow/internal/storage"
	syncerconnector "ragflow/internal/syncer/connector"

	"gorm.io/gorm"
)

type syncMetadataFaultEngine struct {
	fakeChatDocEngine
	err      error
	onUpdate func() error
}

func (e *syncMetadataFaultEngine) UpdateMetadata(context.Context, string, string, map[string]interface{}, string) error {
	if e.onUpdate != nil {
		return e.onUpdate()
	}
	return e.err
}

func setupSyncDocumentTest(t *testing.T, existing bool) (*DocumentService, *gorm.DB, *fakeUploadStorage, service.DocumentUpsertInput) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-sync", "tenant-1", 0, 0, 0)
	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-sync")
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeUploadStorage()
	factory := storage.GetStorageFactory()
	original := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(original) })
	svc := testDocumentService(t)
	input := service.DocumentUpsertInput{
		TaskContext: service.SyncTaskContext{Knowledgebase: *kb, Connector: entity.Connector{TenantID: "tenant-1"}},
		SourceType:  "github_connector-1",
		DocumentID:  "doc-sync",
		SourceDocument: syncerconnector.SourceDocument{
			SourceID: "source-1", SemanticIdentifier: "source", Extension: ".txt",
			Fingerprint: "new-hash", Blob: []byte("new content"),
		},
	}
	if existing {
		old := input
		old.SourceDocument.Fingerprint = "old-hash"
		old.SourceDocument.Blob = []byte("old content")
		if _, err := svc.Upsert(t.Context(), old); err != nil {
			t.Fatalf("seed existing document: %v", err)
		}
	}
	return svc, db, store, input
}

func assertSyncDocumentState(t *testing.T, db *gorm.DB, store *fakeUploadStorage, input service.DocumentUpsertInput, hash string) *entity.Document {
	t.Helper()
	doc, err := dao.NewDocumentDAO().GetByID(t.Context(), db, input.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ContentHash == nil || *doc.ContentHash != hash {
		t.Fatalf("fingerprint = %v, want %q", doc.ContentHash, hash)
	}
	if doc.Location == nil {
		t.Fatal("document has no storage location")
	}
	blob, err := store.Get(t.Context(), doc.KbID, *doc.Location)
	if err != nil || string(blob) != string(input.SourceDocument.Blob) {
		t.Fatalf("published blob = %q, err = %v", blob, err)
	}
	return doc
}

func TestSyncDocumentDownstreamFailurePreservesPublishedBlobAndRetries(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, fault := range []string{"metadata", "fingerprint", "file-manager"} {
			if !existing && fault == "file-manager" {
				continue // Insert link failures roll the document back; covered separately.
			}
			name := "insert/" + fault
			if existing {
				name = "update/" + fault
			}
			t.Run(name, func(t *testing.T) {
				svc, db, store, input := setupSyncDocumentTest(t, existing)
				faultErr := errors.New("injected " + fault + " failure")
				engine := &syncMetadataFaultEngine{}
				if fault == "metadata" {
					input.SourceDocument.Metadata = map[string]interface{}{"category": "changed"}
					engine.err = faultErr
					svc.docEngine = engine
					svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), engine)
				} else {
					if err := db.Callback().Update().Before("gorm:update").Register("sync-fault", func(tx *gorm.DB) {
						updates, _ := tx.Statement.Dest.(map[string]interface{})
						if fault == "file-manager" && tx.Statement.Table == "file" ||
							fault == "fingerprint" && tx.Statement.Table == "document" && updates["content_hash"] == "new-hash" {
							tx.AddError(faultErr)
						}
					}); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := svc.Upsert(t.Context(), input); !errors.Is(err, faultErr) {
					t.Fatalf("Upsert error = %v, want injected failure", err)
				}
				assertSyncDocumentState(t, db, store, input, "")
				// Both runner-level and connector-level skip checks use this persisted hash.
				fingerprints, err := service.NewGormDocumentStore().ListFingerprintsBySourceType(t.Context(), input.TaskContext.Knowledgebase.ID, input.SourceType)
				if err != nil || fingerprints[input.DocumentID] != "" {
					t.Fatalf("pending fingerprint snapshot = %v, err = %v", fingerprints, err)
				}
				engine.err = nil
				if fault != "metadata" {
					if err := db.Callback().Update().Remove("sync-fault"); err != nil {
						t.Fatal(err)
					}
				}
				result, err := svc.Upsert(t.Context(), input)
				if err != nil || result.Action != service.DocumentActionUpdated {
					t.Fatalf("retry = %+v, err = %v", result, err)
				}
				doc := assertSyncDocumentState(t, db, store, input, "new-hash")
				location := *doc.Location
				count := len(store.objects)
				result, err = svc.Upsert(t.Context(), input)
				if err != nil || result.Action != service.DocumentActionSkipped {
					t.Fatalf("unchanged successful sync = %+v, err = %v", result, err)
				}
				if doc = assertSyncDocumentState(t, db, store, input, "new-hash"); *doc.Location != location || len(store.objects) != count {
					t.Fatal("unchanged successful sync rewrote the blob")
				}
			})
		}
	}
}

func TestSyncDocumentRunningParseRetryAndSourceRevert(t *testing.T) {
	for _, revert := range []bool{false, true} {
		name := "same-revision"
		if revert {
			name = "source-reverted"
		}
		t.Run(name, func(t *testing.T) {
			svc, db, store, input := setupSyncDocumentTest(t, true)
			input.AutoParse = true
			insertTestIngestionTaskWithStatus(t, "old-task", "tenant-1", input.DocumentID, "kb-sync", common.RUNNING)
			for attempt := 0; attempt < 2; attempt++ {
				if _, err := svc.Upsert(t.Context(), input); err == nil || !strings.Contains(err.Error(), common.RUNNING) {
					t.Fatalf("running parse Upsert = %v", err)
				}
				assertSyncDocumentState(t, db, store, input, "")
			}
			if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "old-task").Update("status", common.COMPLETED).Error; err != nil {
				t.Fatal(err)
			}
			if revert {
				input.SourceDocument.Fingerprint = "old-hash"
				input.SourceDocument.Blob = []byte("old content")
			}
			svc.purgeTaskState = func(context.Context, string) error { return nil }
			publisher := &recordingTaskPublisher{}
			svc.ingestionTaskSvc.SetTaskPublisher(publisher)
			result, err := svc.Upsert(t.Context(), input)
			if err != nil || result.Action != service.DocumentActionUpdated {
				t.Fatalf("terminal task retry = %+v, err = %v", result, err)
			}
			assertSyncDocumentState(t, db, store, input, input.SourceDocument.Fingerprint)
			if len(publisher.messages) != 1 || publisher.messages[0].TaskID == "old-task" {
				t.Fatalf("replacement parse publications = %#v", publisher.messages)
			}
			if result, err = svc.Upsert(t.Context(), input); err != nil || result.Action != service.DocumentActionSkipped || len(publisher.messages) != 1 {
				t.Fatalf("unchanged parse retry = %+v, err = %v, publications = %d", result, err, len(publisher.messages))
			}
		})
	}
}

func TestSyncDocumentInsertLinkRollbackCleanup(t *testing.T) {
	for _, rollbackFails := range []bool{false, true} {
		name := "rollback-success"
		if rollbackFails {
			name = "rollback-failure"
		}
		t.Run(name, func(t *testing.T) {
			svc, db, store, input := setupSyncDocumentTest(t, false)
			if err := db.Callback().Create().Before("gorm:create").Register("link-fault", func(tx *gorm.DB) {
				if file, ok := tx.Statement.Dest.(*entity.File); ok && file.Type != "folder" {
					tx.AddError(errors.New("file link failed"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			if rollbackFails {
				if err := db.Callback().Delete().Before("gorm:delete").Register("rollback-fault", func(tx *gorm.DB) {
					if tx.Statement.Table == "document" {
						tx.AddError(errors.New("rollback failed"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := svc.Upsert(t.Context(), input); err == nil {
				t.Fatal("expected file link failure")
			}
			if rollbackFails {
				assertSyncDocumentState(t, db, store, input, "")
			} else if len(store.objects) != 0 {
				t.Fatalf("rolled-back document left %d unreferenced blobs", len(store.objects))
			}
		})
	}
}

func TestSyncDocumentConcurrentPublicationCannotAcknowledgeAnotherRevision(t *testing.T) {
	svc, db, store, input := setupSyncDocumentTest(t, true)
	input.SourceDocument.Metadata = map[string]interface{}{"category": "changed"}
	replacementLocation := "sync/concurrent.txt"
	engine := &syncMetadataFaultEngine{onUpdate: func() error {
		if err := store.Put(t.Context(), "kb-sync", replacementLocation, []byte("another revision")); err != nil {
			return err
		}
		return db.Model(&entity.Document{}).Where("id = ?", input.DocumentID).
			Updates(map[string]interface{}{"location": replacementLocation, "content_hash": ""}).Error
	}}
	svc.docEngine = engine
	svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), engine)
	if _, err := svc.Upsert(t.Context(), input); err == nil || !strings.Contains(err.Error(), "changed before") {
		t.Fatalf("concurrent publication Upsert = %v", err)
	}
	input.SourceDocument.Blob = []byte("another revision")
	doc := assertSyncDocumentState(t, db, store, input, "")
	if *doc.Location != replacementLocation {
		t.Fatalf("location = %q, want concurrent revision", *doc.Location)
	}
}

func TestSyncDocumentCleanupPreservesBlobWhenReferenceLookupFails(t *testing.T) {
	svc, db, store, input := setupSyncDocumentTest(t, true)
	store.afterPut = func() {
		if err := db.Callback().Query().Before("gorm:query").Register("lookup-fault", func(tx *gorm.DB) {
			if tx.Statement.Table == "document" {
				tx.AddError(errors.New("reference lookup unavailable"))
			}
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Callback().Update().Before("gorm:update").Register("update-fault", func(tx *gorm.DB) {
		if tx.Statement.Table == "document" {
			tx.AddError(errors.New("update unavailable"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Upsert(t.Context(), input); err == nil {
		t.Fatal("expected update failure")
	}
	if len(store.objects) != 2 {
		t.Fatalf("unknown publication state removed a blob: objects = %d", len(store.objects))
	}
}

func TestSyncDocumentSerializesMetadataAcrossRevisions(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new-document"
		if existing {
			name = "existing-document"
		}
		t.Run(name, func(t *testing.T) {
			first, db, store, input := setupSyncDocumentTest(t, existing)
			second := testDocumentService(t)
			firstEntered := make(chan struct{})
			releaseFirst := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
			defer release()
			var mu sync.Mutex
			metadata := ""
			firstEngine := &syncMetadataFaultEngine{onUpdate: func() error {
				close(firstEntered)
				<-releaseFirst
				mu.Lock()
				metadata = "first"
				mu.Unlock()
				return nil
			}}
			secondEngine := &syncMetadataFaultEngine{onUpdate: func() error {
				mu.Lock()
				metadata = "second"
				mu.Unlock()
				return nil
			}}
			first.docEngine = firstEngine
			first.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), firstEngine)
			second.docEngine = secondEngine
			second.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), secondEngine)
			input.SourceDocument.Metadata = map[string]interface{}{"revision": "first"}
			firstResult := make(chan error, 1)
			go func() { _, err := first.Upsert(t.Context(), input); firstResult <- err }()
			select {
			case <-firstEntered:
			case <-time.After(5 * time.Second):
				t.Fatal("first revision did not reach metadata")
			}
			next := input
			next.SourceDocument.Fingerprint = "second-hash"
			next.SourceDocument.Blob = []byte("second content")
			next.SourceDocument.Metadata = map[string]interface{}{"revision": "second"}
			secondStarted := make(chan struct{})
			secondResult := make(chan error, 1)
			go func() {
				close(secondStarted)
				_, err := second.Upsert(t.Context(), next)
				secondResult <- err
			}()
			<-secondStarted
			select {
			case err := <-secondResult:
				t.Fatalf("second revision escaped the first revision's metadata barrier: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			release()
			for _, result := range []<-chan error{firstResult, secondResult} {
				select {
				case err := <-result:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("serialized revision did not finish")
				}
			}
			assertSyncDocumentState(t, db, store, next, "second-hash")
			mu.Lock()
			defer mu.Unlock()
			if metadata != "second" {
				t.Fatalf("acknowledged second revision has stale metadata %q", metadata)
			}
		})
	}
}

func TestSyncDocumentAcknowledgesCompletedWorkAfterCancellation(t *testing.T) {
	svc, db, store, input := setupSyncDocumentTest(t, true)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	engine := &syncMetadataFaultEngine{onUpdate: func() error { cancel(); return nil }}
	svc.docEngine = engine
	svc.metadataSvc = service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), engine)
	input.SourceDocument.Metadata = map[string]interface{}{"revision": "new"}
	result, err := svc.Upsert(ctx, input)
	if err != nil || result.Action != service.DocumentActionUpdated {
		t.Fatalf("completed dependent work was not acknowledged: result = %+v err = %v", result, err)
	}
	assertSyncDocumentState(t, db, store, input, "new-hash")
}

func TestSyncDocumentAcknowledgesParseEnqueueAfterCancellation(t *testing.T) {
	svc, db, store, input := setupSyncDocumentTest(t, true)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	publisher := &recordingTaskPublisher{}
	svc.ingestionTaskSvc.SetTaskPublisher(publisher)
	input.AutoParse = true
	// Cancel at the final fingerprint write, after StartParseDocuments has
	// completed both publication and its scheduled-state bookkeeping.
	if err := db.Callback().Update().Before("gorm:update").Register("cancel-before-ack", func(tx *gorm.DB) {
		updates, _ := tx.Statement.Dest.(map[string]interface{})
		if tx.Statement.Table == "document" && updates["content_hash"] == input.SourceDocument.Fingerprint {
			cancel()
		}
	}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Upsert(ctx, input)
	if err != nil || result.Action != service.DocumentActionUpdated {
		t.Fatalf("successful enqueue was not acknowledged: result = %+v err = %v", result, err)
	}
	assertSyncDocumentState(t, db, store, input, "new-hash")
	if len(publisher.messages) != 1 {
		t.Fatalf("parse publications = %d, want 1", len(publisher.messages))
	}
	result, err = svc.Upsert(t.Context(), input)
	if err != nil || result.Action != service.DocumentActionSkipped || len(publisher.messages) != 1 {
		t.Fatalf("completed revision retried enqueue: result = %+v err = %v publications = %d", result, err, len(publisher.messages))
	}
}
