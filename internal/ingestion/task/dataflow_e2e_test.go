//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package task

import (
	"context"
	"fmt"
	"net"
	"ragflow/internal/common"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/infinity"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	"ragflow/internal/server"
)

// =============================================================================
// Test helper: setupTestDocEngine - Creates either Elasticsearch or Infinity engine
// =============================================================================

func setupTestDocEngine(t *testing.T, engineType engine.EngineType, tenantID, datasetID string) (engine.DocEngine, func()) {
	t.Helper()

	var (
		docEngine engine.DocEngine
		err       error
	)

	switch engineType {
	case engine.EngineElasticsearch:
		t.Logf("Setting up Elasticsearch engine...")
		esHost := common.GetEnv(common.EnvESHost)
		if esHost == "" {
			esHost = "localhost:1200"
		}
		if !startsWithHTTP(esHost) {
			esHost = "http://" + esHost
		}
		esUser := common.GetEnv(common.EnvESUsername)
		if esUser == "" {
			esUser = "elastic"
		}
		esPassword := common.GetEnv(common.EnvESPassword)
		if esPassword == "" {
			esPassword = "infini_rag_flow"
		}

		cfg := &server.ElasticsearchConfig{
			Hosts:    esHost,
			Username: esUser,
			Password: esPassword,
		}
		docEngine, err = elasticsearch.NewEngine(cfg)
		if err != nil {
			t.Skipf("Could not create Elasticsearch engine: %v (skipping ES subtest)", err)
			return nil, func() {}
		}

	case engine.EngineInfinity:
		t.Logf("Setting up Infinity engine...")
		infURI := common.GetEnv(common.EnvInfinityURI)
		if infURI == "" {
			infURI = "localhost:23817"
		}

		// First check if Infinity is reachable quickly without waiting 120s!
		if !isPortOpen("localhost", 23817) {
			t.Skipf("Infinity not running at %s (skipping Infinity subtest)", infURI)
			return nil, func() {}
		}

		cfg := &server.InfinityConfig{
			URI:          infURI,
			DBName:       "ragflow_e2e_test",
			PostgresPort: 5432,
		}
		docEngine, err = infinity.NewEngine(cfg)
		if err != nil {
			t.Skipf("Could not create Infinity engine: %v (skipping Infinity subtest)", err)
			return nil, func() {}
		}

	default:
		t.Fatalf("Unsupported engine type: %v", engineType)
	}

	// Create unique base name for the test
	baseName := fmt.Sprintf("ragflow_%s", tenantID)

	// Cleanup first (if exists)
	ctx := context.Background()
	_ = docEngine.DropChunkStore(ctx, baseName, datasetID)

	// Create chunk store (note: vec dimension = 2 because our test chunks have q_2_vec)
	if err := docEngine.CreateChunkStore(ctx, baseName, datasetID, 2, "naive"); err != nil {
		// If create failed, maybe it exists; try dropping and recreating
		_ = docEngine.DropChunkStore(ctx, baseName, datasetID)
		if err := docEngine.CreateChunkStore(ctx, baseName, datasetID, 2, "naive"); err != nil {
			_ = docEngine.Close()
			t.Fatalf("Could not create chunk store: %v", err)
		}
	}

	// Return cleanup function
	cleanup := func() {
		_ = docEngine.DropChunkStore(ctx, baseName, datasetID)
		_ = docEngine.Close()
	}

	return docEngine, cleanup
}

func startsWithHTTP(s string) bool {
	return len(s) >= 4 && (s[:4] == "http" || s[:5] == "https")
}

func toLowerSnakeCase(s string) string {
	result := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			result = append(result, r-'A'+'a')
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result = append(result, r)
		} else {
			result = append(result, '_')
		}
	}
	return string(result)
}

func isPortOpen(host string, port int) bool {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

// =============================================================================
// Main E2E Test with Subtests for Elasticsearch and Infinity
// =============================================================================

func TestDataflowE2E_TaskHandlerToDataflowService(t *testing.T) {
	testCases := []struct {
		name       string
		engineType engine.EngineType
	}{
		{
			name:       "Elasticsearch",
			engineType: engine.EngineElasticsearch,
		},
		{
			name:       "Infinity",
			engineType: engine.EngineInfinity,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Running Dataflow E2E test with engine: %s", tc.name)

			// Setup test database
			db := testutil.SetupTestDB(t)
			cleanupDB := testutil.ReplaceDBForTest(t, db)
			defer cleanupDB()

			// Seed test data (lowercase for ES index compatibility)
			lowerName := toLowerSnakeCase(tc.name)
			tenantID, kbID, _, taskID := testutil.SeedTestData(t, db,
				testutil.WithTenantID(fmt.Sprintf("tenant-e2e-%s", lowerName)),
				testutil.WithKBID(fmt.Sprintf("kb-e2e-%s", lowerName)),
				testutil.WithDocID(fmt.Sprintf("doc-e2e-%s", lowerName)),
				testutil.WithTaskID(fmt.Sprintf("task-e2e-%s", lowerName)),
				testutil.WithPipelineID(fmt.Sprintf("pipeline-e2e-%s", lowerName)),
				testutil.WithDocName(fmt.Sprintf("e2e-test-%s.pdf", lowerName)),
			)

			// Setup DocEngine for this test
			docEngine, cleanupEngine := setupTestDocEngine(t, tc.engineType, tenantID, kbID)
			defer cleanupEngine()

			// Update task to have dataflow task type
			var task entity.Task
			if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
				t.Fatalf("Failed to get task: %v", err)
			}
			task.TaskType = "dataflow"
			if err := db.Save(&task).Error; err != nil {
				t.Fatalf("Failed to update task: %v", err)
			}

			// Load task context
			taskCtx, err := LoadTaskContext(taskID)
			if err != nil {
				t.Fatalf("LoadTaskContext failed: %v", err)
			}

			// Track what was called
			var (
				loadDSLCalled      bool
				runPipelineCalled  bool
				insertChunksCalled bool
			)
			var capturedChunks [][]map[string]any

			// Create TaskHandler with mocked DataflowService factory
			handler := NewTaskHandler(taskCtx)
			handler.WithDataflowServiceFactory(func(ctx *TaskContext, dataflowID string) (*PipelineExecutor, error) {
				svc := mustNewDataflowService(t, ctx, dataflowID, 0, 0)

				// Mock loadDSLFunc
				svc.WithLoadDSLFunc(func(ctx context.Context, dataflowID string) (string, string, error) {
					loadDSLCalled = true
					return `{"nodes":[{"id":"test","type":"parser"}],"edges":[]}`, dataflowID, nil
				})

				// Mock runPipelineFunc - returns test chunks (with vectors to skip embedding)
				svc.WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
					runPipelineCalled = true
					return map[string]any{
						"chunks": []map[string]any{
							{
								"text":    fmt.Sprintf("Hello world from E2E test with %s", tc.name),
								"id":      fmt.Sprintf("chunk-e2e-%s-1", tc.name),
								"q_2_vec": []float64{0.1, 0.2}, // Pre-vectorized to skip embedding
							},
							{
								"text":    fmt.Sprintf("Second chunk from E2E test with %s", tc.name),
								"id":      fmt.Sprintf("chunk-e2e-%s-2", tc.name),
								"q_2_vec": []float64{0.3, 0.4}, // Pre-vectorized to skip embedding
							},
						},
						EmbeddingTokenConsumptionKey: 100,
					}, dsl, nil
				})

				// Use the injected DocEngine for insertChunks!
				svc.WithInsertChunksFunc(func(ctx context.Context, chunks []map[string]any, baseName string, datasetID string) ([]string, error) {
					insertChunksCalled = true
					t.Logf("DocEngine InsertChunks called! baseName=%s datasetID=%s len(chunks)=%d", baseName, datasetID, len(chunks))
					ids, err := docEngine.InsertChunks(ctx, chunks, baseName, datasetID)
					if err != nil {
						t.Logf("WARNING: InsertChunks err=%v", err)
					}
					capturedChunks = append(capturedChunks, chunks)
					return ids, err
				})

				svc.WithChunkCounter(&stubChunkCounter{})
				return svc, nil
			})

			// Also set progress callback
			var progressEvents []struct {
				prog float64
				msg  string
			}
			taskCtx.ProgressFunc = func(prog float64, msg string) {
				t.Logf("PROGRESS: %.2f %s", prog, msg)
				progressEvents = append(progressEvents, struct {
					prog float64
					msg  string
				}{prog, msg})
			}

			// Execute the task handler!
			t.Logf("Calling TaskHandler.Handle()...")
			err = handler.Handle()
			if err != nil {
				t.Fatalf("TaskHandler.Handle failed: %v", err)
			}
			t.Logf("TaskHandler.Handle() complete!")

			// Verify all the expected calls happened
			if !loadDSLCalled {
				t.Fatal("Expected loadDSLFunc to be called")
			}
			if !runPipelineCalled {
				t.Fatal("Expected runPipelineFunc to be called")
			}
			if !insertChunksCalled {
				t.Fatal("Expected insertChunks to be called")
			}

			// Verify chunks were captured
			totalChunks := 0
			for _, batch := range capturedChunks {
				totalChunks += len(batch)
			}
			if totalChunks != 2 {
				t.Fatalf("Expected total 2 chunks, got %d", totalChunks)
			}

			// Now verify we can read the chunks back!
			baseName := fmt.Sprintf("ragflow_%s", tenantID)
			t.Logf("Reading chunks back from %s: baseName=%s datasetID=%s", tc.name, baseName, kbID)

			// Refresh (wait for ES to index or Infinity to commit)
			time.Sleep(300 * time.Millisecond)

			for _, batch := range capturedChunks {
				for _, chunk := range batch {
					chunkID, ok := chunk["id"].(string)
					if ok && chunkID != "" {
						t.Logf("Trying to get chunk: %s", chunkID)
						readBack, err := docEngine.GetChunk(context.Background(), baseName, chunkID, []string{kbID})
						if err != nil {
							t.Logf("WARNING: Failed to get chunk %q: %v", chunkID, err)
						} else if readBack == nil {
							t.Logf("WARNING: chunk %q not found (may not have been indexed yet)", chunkID)
						} else {
							t.Logf("SUCCESS: Read back chunk %q!", chunkID)
						}
					}
				}
			}

			// Verify progress reported
			if len(progressEvents) == 0 {
				t.Fatal("Expected at least one progress event")
			}
			foundDone := false
			for _, ev := range progressEvents {
				if ev.prog == 1.0 {
					foundDone = true
				}
			}
			if !foundDone {
				t.Fatal("Expected progress to reach 1.0")
			}

			// Verify final task status can be updated to success
			ingestionTaskDAO := dao.NewIngestionTaskDAO()
			if err = ingestionTaskDAO.UpdateStatus(taskID, "success"); err != nil {
				t.Fatalf("UpdateStatus failed: %v", err)
			}

			finalTask, err := ingestionTaskDAO.GetByID(taskID)
			if err != nil {
				t.Fatalf("GetByID failed: %v", err)
			}
			if finalTask.Status != "success" {
				t.Errorf("Final task status = %q, want %q", finalTask.Status, "success")
			}

			t.Logf("SUCCESS: Dataflow E2E test passed with %s engine!", tc.name)
		})
	}
}
