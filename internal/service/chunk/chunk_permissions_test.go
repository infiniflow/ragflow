package chunk

import (
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service"
)

func TestChunkManagementDatasetPermissions(t *testing.T) {
	for _, tt := range []struct {
		name       string
		userID     string
		permission entity.TenantPermission
		allowed    bool
	}{
		{"owner private", "owner", entity.TenantPermissionMe, true},
		{"member private", "member", entity.TenantPermissionMe, false},
		{"member team", "member", entity.TenantPermissionTeam, true},
		{"unrelated team", "other", entity.TenantPermissionTeam, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupChunkTestDB(t)
			pushChunkTestDB(t, db)
			insertChunkTestUserTenant(t, "member", "owner")
			insertChunkTestKB(t, "kb-1", "owner")
			insertChunkTestDoc(t, "doc-1", "kb-1")
			if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").
				Update("permission", string(tt.permission)).Error; err != nil {
				t.Fatalf("set KB permission: %v", err)
			}

			ctx := t.Context()
			for _, op := range []struct {
				name string
				run  func(*ChunkService) error
			}{
				{"get", func(s *ChunkService) error {
					s.docEngine = &getChunkTestEngine{chunk: map[string]interface{}{
						"id": "chunk-1", "doc_id": "doc-1", "content_with_weight": "body",
					}}
					_, err := s.Get(ctx, &service.GetChunkRequest{
						DatasetID: "kb-1", DocumentID: "doc-1", ChunkID: "chunk-1",
					}, tt.userID)
					return err
				}},
				{"list", func(s *ChunkService) error {
					s.docEngine = &listChunksSearchEngine{}
					_, err := s.List(ctx, &service.ListChunksRequest{DatasetID: "kb-1", DocID: "doc-1"}, tt.userID)
					return err
				}},
				{"switch", func(s *ChunkService) error {
					s.docEngine = &updateChunkTestEngine{}
					return s.SwitchChunks(ctx, tt.userID, "kb-1", "doc-1", 0, []string{"chunk-1"})
				}},
				{"update", func(s *ChunkService) error {
					s.docEngine = &updateChunkTestEngine{existingChunk: map[string]interface{}{
						"id": "chunk-1", "doc_id": "doc-1", "content_with_weight": "body",
					}}
					return s.UpdateChunk(ctx, &service.UpdateChunkRequest{
						DatasetID: "kb-1", DocumentID: "doc-1", ChunkID: "chunk-1",
					}, tt.userID)
				}},
				{"remove", func(s *ChunkService) error {
					s.docEngine = &parseTestDocEngine{}
					_, err := s.RemoveChunks(ctx, &service.RemoveChunksRequest{DocID: "doc-1", DeleteAll: true}, tt.userID)
					return err
				}},
			} {
				t.Run(op.name, func(t *testing.T) {
					svc := &ChunkService{
						kbDAO:             dao.NewKnowledgebaseDAO(),
						documentDAO:       dao.NewDocumentDAO(),
						markWikiDirtyFunc: func(string, string, string, []string) {},
					}
					err := op.run(svc)
					if tt.allowed {
						if err != nil {
							t.Fatalf("authorized operation failed: %v", err)
						}
					} else if !permissionresponse.IsPermissionError(err) {
						t.Fatalf("unauthorized operation error = %v, want access denied", err)
					}
				})
			}
		})
	}
}
