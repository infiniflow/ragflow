//go:build integration

package document

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/elasticsearch"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion"
	"ragflow/internal/ingestion/task"
	"ragflow/internal/server/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Requires an EMPTY throwaway MySQL database and a real Elasticsearch server:
//
//	RAGFLOW_TEST_MYSQL_DSN=... RAGFLOW_TEST_ES_URL=http://localhost:9200 \
//	  bash build.sh --test-integration-go ./internal/service/document -run TestDocumentPublicationRejectsActiveStatusChanges
//
// Parser output is the input fixture. Publication, both status entry points,
// MySQL task checks and ES writes/searches run their production code. No
// externally prepared document, timing script or access token is required.
func TestDocumentPublicationRejectsActiveStatusChanges(t *testing.T) {
	dsn, esURL := os.Getenv("RAGFLOW_TEST_MYSQL_DSN"), os.Getenv("RAGFLOW_TEST_ES_URL")
	if dsn == "" || esURL == "" {
		t.Skip("requires RAGFLOW_TEST_MYSQL_DSN (empty test database) and RAGFLOW_TEST_ES_URL")
	}
	if err := common.InitLogger("error", common.FileOutput{}, "availability_integration"); err != nil {
		t.Fatal(err)
	}
	openDB := func() *gorm.DB {
		t.Helper()
		db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		pool.SetMaxOpenConns(4)
		t.Cleanup(func() { _ = pool.Close() })
		return db
	}
	apiDB, publisherDB := openDB(), openDB()
	models := []any{&entity.User{}, &entity.UserTenant{}, &entity.Tenant{}, &entity.Knowledgebase{}, &entity.Document{}, &entity.IngestionTask{}}
	for _, model := range models {
		if apiDB.Migrator().HasTable(model) {
			t.Fatal("refusing to modify an existing schema: use an empty throwaway database")
		}
	}
	t.Cleanup(func() {
		for i := len(models) - 1; i >= 0; i-- {
			if err := apiDB.Migrator().DropTable(models[i]); err != nil {
				t.Error(err)
			}
		}
	})
	if err := apiDB.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	pushServiceDB(t, apiDB)

	for _, entry := range []string{"single", "batch"} {
		for _, desired := range []int{0, 1} {
			for _, heldBatch := range []int32{1, 3} {
				t.Run(fmt.Sprintf("%s/status=%d/batch=%d", entry, desired, heldBatch), func(t *testing.T) {
					ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
					defer cancel()
					arrived, release := make(chan struct{}), make(chan struct{})
					var workers sync.WaitGroup
					var releaseOnce sync.Once
					releaseBulk := func() { releaseOnce.Do(func() { close(release) }) }
					defer func() {
						releaseBulk()
						cancel()
						workers.Wait()
					}()
					upstream, err := url.Parse(esURL)
					if err != nil {
						t.Fatal(err)
					}
					forward := httputil.NewSingleHostReverseProxy(upstream)
					var batches atomic.Int32
					proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if strings.HasSuffix(r.URL.Path, "/_bulk") {
							if batches.Add(1) == heldBatch {
								close(arrived)
								select {
								case <-release:
								case <-ctx.Done():
									http.Error(w, "test bulk wait expired", http.StatusGatewayTimeout)
									return
								}
							}
						}
						forward.ServeHTTP(w, r)
					}))
					t.Cleanup(proxy.Close)
					es, err := elasticsearch.NewEngine(ctx, config.ElasticsearchConfig{
						Hosts: proxy.URL, Username: os.Getenv("RAGFLOW_TEST_ES_USERNAME"), Password: os.Getenv("RAGFLOW_TEST_ES_PASSWORD"),
					})
					if err != nil {
						t.Fatal(err)
					}
					tenantID := fmt.Sprintf("avail_%x", time.Now().UnixNano())
					indexName := "ragflow_" + tenantID
					t.Cleanup(func() {
						cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
						defer stop()
						if err := es.DropChunkStore(cleanupCtx, indexName, "kb-1"); err != nil {
							t.Error(err)
						}
						apiDB.Where("document_id = ?", "doc-1").Delete(&entity.IngestionTask{})
						apiDB.Where("kb_id = ?", "kb-1").Delete(&entity.Document{})
						apiDB.Where("id = ?", "kb-1").Delete(&entity.Knowledgebase{})
						apiDB.Where("tenant_id = ?", tenantID).Delete(&entity.UserTenant{})
					})
					if err := es.CreateChunkStore(ctx, indexName, "kb-1", 2, "general", "English"); err != nil {
						t.Fatal(err)
					}
					insertTestKB(t, "kb-1", tenantID, 2, 0, 0)
					insertTestDoc(t, "doc-1", "kb-1", 0, 0)
					insertTestDoc(t, "doc-2", "kb-1", 0, 0)
					insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.RUNNING)
					name, initial := "visibility.txt", strconv.Itoa(1-desired)
					if err := apiDB.Model(&entity.Document{}).Where("id = ?", "doc-1").Updates(map[string]any{"name": name, "status": initial}).Error; err != nil {
						t.Fatal(err)
					}
					var doc entity.Document
					if err := publisherDB.Where("id = ?", "doc-1").Take(&doc).Error; err != nil {
						t.Fatal(err)
					}
					chunks := []map[string]any{
						{"text": "first source", "q_2_vec": []float64{1, 0}},
						{"text": "second source", "q_2_vec": []float64{1, 0}},
						{"text": "final source", "q_2_vec": []float64{1, 0}},
					}
					executor, err := task.NewPipelineExecutor(&task.TaskContext{
						Doc: doc, KB: entity.Knowledgebase{ID: "kb-1", TenantID: tenantID}, Tenant: entity.Tenant{ID: tenantID},
					}, "publication-fixture", 1, &ingestion.Deps{DB: publisherDB, Engine: es})
					if err != nil {
						t.Fatal(err)
					}
					executor.WithLoadDSLFunc(func(context.Context, string) (string, string, error) { return "", "", nil }).
						WithRunPipelineFunc(func(context.Context, string) (map[string]any, string, error) {
							return map[string]any{"chunks": chunks}, "", nil
						})
					published := make(chan error, 1)
					workers.Add(1)
					go func() {
						defer workers.Done()
						_, err := executor.Execute(ctx)
						published <- err
					}()
					select {
					case <-arrived:
					case err := <-published:
						t.Fatalf("publication exited before the held batch: %v", err)
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					svc := testDocumentService(t)
					svc.docEngine = es
					actorCtx := documentTestActorContext(ctx, "user-1", tenantID)
					if err := setAvailabilityThroughEntry(actorCtx, svc, entry, strconv.Itoa(desired)); err == nil {
						t.Fatal("active document status change was acknowledged")
					}
					if err := apiDB.Where("id = ?", "doc-1").Take(&doc).Error; err != nil {
						t.Fatal(err)
					}
					if *doc.Status != initial {
						t.Fatalf("rejected request changed document status to %s", *doc.Status)
					}
					// The same batch still updates an unrelated, idle document.
					result, _, err := svc.BatchUpdateDocumentStatus(actorCtx, "kb-1", strconv.Itoa(desired), []string{"doc-1", "doc-2"})
					if err == nil || result["doc-2"].(map[string]string)["status"] != strconv.Itoa(desired) {
						t.Fatalf("mixed batch did not preserve its successful item: result=%v err=%v", result, err)
					}
					releaseBulk()
					select {
					case err := <-published:
						if err != nil {
							t.Fatal(err)
						}
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					// Publication is finished; simulate the enclosing task's completion.
					if err := apiDB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("status", common.COMPLETED).Error; err != nil {
						t.Fatal(err)
					}
					// Counts are deliberately still zero. Both a transition and a
					// same-status retry must reconcile the real index.
					for attempt := range 2 {
						if attempt == 1 {
							// Seed a historical mismatch to exercise same-status repair.
							if err := es.UpdateChunks(ctx, map[string]any{"doc_id": "doc-1"}, map[string]any{"available_int": 1 - desired}, indexName, "kb-1"); err != nil {
								t.Fatal(err)
							}
						}
						if err := setAvailabilityThroughEntry(actorCtx, svc, entry, strconv.Itoa(desired)); err != nil {
							t.Fatal(err)
						}
					}
					if err := apiDB.Where("id = ?", "doc-1").Take(&doc).Error; err != nil {
						t.Fatal(err)
					}
					if *doc.Status != strconv.Itoa(desired) || doc.ChunkNum != 0 {
						t.Fatalf("status=%v count=%d; want current status without count bookkeeping", doc.Status, doc.ChunkNum)
					}
					for _, filter := range []map[string]any{
						{"doc_id": "doc-1"},
						{"doc_id": "doc-1", "available_int": 1},
					} {
						result, err := es.Search(ctx, &enginetypes.SearchRequest{IndexNames: []string{indexName}, KbIDs: []string{"kb-1"}, Limit: 10, Filter: filter})
						if err != nil {
							t.Fatal(err)
						}
						want := 3
						if _, visibleOnly := filter["available_int"]; visibleOnly && desired == 0 {
							want = 0
						}
						if len(result.Chunks) != want {
							t.Fatalf("filter=%v indexed rows=%d, want %d", filter, len(result.Chunks), want)
						}
					}
				})
			}
		}
	}
}
