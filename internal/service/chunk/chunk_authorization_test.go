package chunk

import (
	"context"
	"reflect"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service"
)

// TestChunkManagementAuthorization verifies the shared dataset permission policy
// before any search-engine read or write, using real DAOs and an in-memory DB.
func TestChunkManagementAuthorization(t *testing.T) {
	operations := []struct {
		name      string
		wantCalls []string
		call      func(*ChunkService, string) error
	}{
		{"get", []string{"GetChunk"}, func(s *ChunkService, userID string) error {
			_, err := s.Get(t.Context(), &service.GetChunkRequest{DatasetID: "kb-1", DocumentID: "doc-1", ChunkID: "chunk-1"}, userID)
			return err
		}},
		{"list", []string{"Search"}, func(s *ChunkService, userID string) error {
			_, err := s.List(t.Context(), &service.ListChunksRequest{DatasetID: "kb-1", DocID: "doc-1"}, userID)
			return err
		}},
		{"list by document", []string{"Search"}, func(s *ChunkService, userID string) error {
			_, err := s.List(t.Context(), &service.ListChunksRequest{DocID: "doc-1"}, userID)
			return err
		}},
		{"switch", []string{"UpdateChunks"}, func(s *ChunkService, userID string) error {
			return s.SwitchChunks(t.Context(), userID, "kb-1", "doc-1", 1, []string{"chunk-1"})
		}},
		{"update", []string{"GetChunk", "UpdateChunks"}, func(s *ChunkService, userID string) error {
			available := true
			return s.UpdateChunk(t.Context(), &service.UpdateChunkRequest{DatasetID: "kb-1", DocumentID: "doc-1", ChunkID: "chunk-1", Available: &available}, userID)
		}},
		{"remove", []string{"DeleteChunks"}, func(s *ChunkService, userID string) error {
			_, err := s.RemoveChunks(t.Context(), &service.RemoveChunksRequest{DatasetID: "kb-1", DocID: "doc-1", ChunkIDs: []string{"chunk-1"}}, userID)
			return err
		}},
	}
	cases := []struct {
		name, permission, userID, memberStatus, datasetStatus string
		allowed                                               bool
	}{
		{"owner private", "me", "tenant-1", "1", "1", true},
		{"owner team", "team", "tenant-1", "1", "1", true},
		{"member private", "me", "member-1", "1", "1", false},
		{"member team", "team", "member-1", "1", "1", true},
		{"revoked member team", "team", "member-1", "0", "1", false},
		{"unrelated user team", "team", "unrelated", "1", "1", false},
		{"member unknown permission", "", "member-1", "1", "1", false},
		{"owner invalid dataset", "me", "tenant-1", "1", "0", false},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					db := setupChunkTestDB(t)
					pushChunkTestDB(t, db)
					insertChunkTestUserTenant(t, "tenant-1", "tenant-1")
					if err := db.Create(&entity.UserTenant{
						ID: "membership-1", UserID: "member-1", TenantID: "tenant-1",
						Role: "normal", Status: &tc.memberStatus,
					}).Error; err != nil {
						t.Fatalf("create membership: %v", err)
					}
					insertChunkTestKB(t, "kb-1", "tenant-1")
					insertChunkTestDoc(t, "doc-1", "kb-1")
					if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Updates(map[string]interface{}{
						"permission": tc.permission, "status": tc.datasetStatus,
					}).Error; err != nil {
						t.Fatalf("set dataset permissions: %v", err)
					}
					docEngine := &chunkAuthorizationTestEngine{}
					downstreamCalls := 0
					svc := &ChunkService{
						docEngine: docEngine, kbDAO: dao.NewKnowledgebaseDAO(),
						documentDAO: dao.NewDocumentDAO(),
						decrementChunkStatsFunc: func(string, string, int64, int64, float64) error {
							downstreamCalls++
							return nil
						},
						markWikiDirtyFunc: func(string, string, string, []string) {
							downstreamCalls++
						},
					}
					err := operation.call(svc, tc.userID)
					if tc.allowed {
						if err != nil {
							t.Fatalf("authorized operation failed: %v", err)
						}
						if !reflect.DeepEqual(docEngine.calls, operation.wantCalls) {
							t.Fatalf("engine calls = %v, want %v", docEngine.calls, operation.wantCalls)
						}
						return
					}
					if err == nil {
						t.Fatal("expected unauthorized operation to be rejected")
					}
					if !permissionresponse.IsPermissionError(err) {
						t.Fatalf("expected an authorization error, got %v", err)
					}
					if len(docEngine.calls) != 0 || downstreamCalls != 0 {
						t.Fatalf("unauthorized downstream calls: engine=%v side effects=%d", docEngine.calls, downstreamCalls)
					}
				})
			}
		})
	}
}

// chunkAuthorizationTestEngine records allowed calls beyond the authorization boundary.
// Its unconfigured embedded interface makes every unexpected method fail the test.
type chunkAuthorizationTestEngine struct {
	engine.DocEngine
	calls []string
}

// Search records reads beyond the authorization boundary.
func (e *chunkAuthorizationTestEngine) Search(context.Context, *types.SearchRequest) (*types.SearchResult, error) {
	e.calls = append(e.calls, "Search")
	return &types.SearchResult{}, nil
}

// GetChunk records detail reads and returns the authorized fixture.
func (e *chunkAuthorizationTestEngine) GetChunk(context.Context, string, string, []string) (interface{}, error) {
	e.calls = append(e.calls, "GetChunk")
	return map[string]interface{}{"id": "chunk-1", "doc_id": "doc-1", "kb_id": "kb-1", "content_with_weight": "sample"}, nil
}

// UpdateChunks records mutation calls beyond the authorization boundary.
func (e *chunkAuthorizationTestEngine) UpdateChunks(context.Context, map[string]interface{}, map[string]interface{}, string, string) error {
	e.calls = append(e.calls, "UpdateChunks")
	return nil
}

// DeleteChunks records deletions beyond the authorization boundary.
func (e *chunkAuthorizationTestEngine) DeleteChunks(context.Context, map[string]interface{}, string, string) (int64, error) {
	e.calls = append(e.calls, "DeleteChunks")
	return 1, nil
}
