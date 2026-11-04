//go:build integration

package elasticsearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

// TestUpdateSingleChunkScopeRealES verifies dataset and document isolation
// against a real Elasticsearch service, with a matching-scope positive control.
func TestUpdateSingleChunkScopeRealES(t *testing.T) {
	if common.GetEnv(common.EnvESTest) != "1" {
		t.Skip("requires ES_TEST=1 and a real Elasticsearch service")
	}
	cfg := getESTestConfig()
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{cfg.Hosts}, Username: cfg.Username, Password: cfg.Password,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Avoid NewEngine: its bootstrap writes shared index templates. This
	// experiment only needs the real client and one disposable tenant index.
	engine := &Engine{client: client}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	index := fmt.Sprintf("ragflow_scope_repro_%d", time.Now().UnixNano())
	checkResponse := func(res *esapi.Response, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.IsError() {
			t.Fatalf("ES request failed: %s", res.String())
		}
	}
	checkResponse(client.Indices.Create(index, client.Indices.Create.WithContext(ctx),
		client.Indices.Create.WithBody(strings.NewReader(`{"settings":{"number_of_shards":1,"number_of_replicas":0},"mappings":{"properties":{"id":{"type":"keyword"},"kb_id":{"type":"keyword"},"doc_id":{"type":"keyword"},"available_int":{"type":"integer"}}}}`))))
	t.Logf("temporary tenant index: %s", index)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		res, err := client.Indices.Delete([]string{index}, client.Indices.Delete.WithContext(cleanupCtx))
		if err != nil {
			t.Errorf("cleanup %s: %v", index, err)
			return
		}
		defer res.Body.Close()
		if res.IsError() {
			t.Errorf("cleanup %s: %s", index, res.String())
			return
		}
		t.Logf("deleted temporary index: %s", index)
	})
	chunkID := common.ChunkID("doc-b", "scope reproduction")
	for _, tt := range []struct {
		name, datasetID, documentID string
		wantAvailable               float64
	}{
		{"matching_scope", "kb-b", "doc-b", 0},
		{"wrong_dataset", "kb-a", "doc-b", 1},
		{"wrong_document", "kb-b", "doc-a", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			chunk := map[string]interface{}{
				"id": chunkID, "doc_id": "doc-b", "available_int": 1,
				"content_with_weight": "original body", "obsolete_kwd": "keep until matched",
				"tag_feas": map[string]int{"old_tag": 2},
			}
			if _, err := engine.InsertChunks(ctx, []map[string]interface{}{chunk}, index, "kb-b", ""); err != nil {
				t.Fatal(err)
			}
			if tt.datasetID == "kb-a" {
				got, err := engine.GetChunk(ctx, index, chunkID, []string{"kb-a"})
				if err != nil || got != nil {
					t.Fatalf("scoped read in A = (%v, %v), want (nil, nil)", got, err)
				}
				t.Log("scoped read in A: not found; fixture belongs to B")
			}
			updateErr := engine.UpdateChunks(ctx,
				map[string]interface{}{"id": chunkID, "doc_id": tt.documentID},
				map[string]interface{}{
					"id": chunkID, "available_int": 0, "content_with_weight": "chunk's body\nsecond line",
					"tag_feas": map[string]int{"new_tag": 3}, "remove": "obsolete_kwd",
				}, index, tt.datasetID)
			res, err := client.Get(index, chunkID, client.Get.WithContext(ctx))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.IsError() {
				t.Fatalf("read fixture by _id: %s", res.String())
			}
			var stored struct {
				Source map[string]interface{} `json:"_source"`
			}
			if err := json.NewDecoder(res.Body).Decode(&stored); err != nil {
				t.Fatal(err)
			}
			t.Logf("requested kb_id=%s doc_id=%s; update error=%v; before available_int=1; after=%v",
				tt.datasetID, tt.documentID, updateErr, stored.Source)
			if stored.Source["kb_id"] != "kb-b" || stored.Source["doc_id"] != "doc-b" {
				t.Fatalf("fixture identity changed: %v", stored.Source)
			}
			if stored.Source["available_int"] != tt.wantAvailable {
				t.Errorf("scope violation: available_int=%v, want %v", stored.Source["available_int"], tt.wantAvailable)
			}
			if tt.name == "matching_scope" && updateErr != nil {
				t.Fatalf("positive control update failed: %v", updateErr)
			}
			if tt.name != "matching_scope" && !errors.Is(updateErr, types.ErrDocumentNotFound) {
				t.Fatalf("mismatched scope error = %v, want ErrDocumentNotFound", updateErr)
			}
			wantContent, wantTag, wantScore := "original body", "old_tag", float64(2)
			if tt.name == "matching_scope" {
				wantContent, wantTag, wantScore = "chunk's body\nsecond line", "new_tag", 3
				if _, exists := stored.Source["obsolete_kwd"]; exists {
					t.Error("matched update did not remove obsolete_kwd")
				}
			} else if stored.Source["obsolete_kwd"] != "keep until matched" {
				t.Error("mismatched update removed obsolete_kwd")
			}
			if stored.Source["content_with_weight"] != wantContent || stored.Source["id"] != chunkID {
				t.Errorf("content or chunk id changed unexpectedly: %v", stored.Source)
			}
			tags, ok := stored.Source["tag_feas"].(map[string]interface{})
			if !ok || len(tags) != 1 || tags[wantTag] != wantScore {
				t.Errorf("tag_feas = %v, want only %s=%v", stored.Source["tag_feas"], wantTag, wantScore)
			}
		})
	}
}
