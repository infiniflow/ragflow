package document

import (
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// TestUpdateDatasetDocumentColumnPatchKeepsOtherParameters: changing a column
// role is a narrow edit. The general parser_config path rebuilds the document's
// configuration from the current DSL, which would reset every parameter this
// request never mentioned — including ones the document had set for itself.
func TestUpdateDatasetDocumentColumnPatchKeepsOtherParameters(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "sales.xlsx", 0, 0)

	before := entity.JSONMap{
		"TableChunker:FastFoxesJump": map[string]interface{}{
			"column_mode":     "manual",
			"column_roles":    map[string]interface{}{"金额": "metadata"},
			"enable_children": true,
		},
		"Tokenizer:SomeNode": map[string]interface{}{"chunk_token_size": 512},
	}
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").
		Update("parser_config", before).Error; err != nil {
		t.Fatalf("seed parser_config: %v", err)
	}

	svc := testDocumentService(t)
	_, code, err := svc.UpdateDatasetDocument(t.Context(), "tenant-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{ParserConfig: map[string]interface{}{
			"TableChunker:FastFoxesJump": map[string]interface{}{
				"column_roles": map[string]interface{}{"金额": "indexing"},
			},
		}},
		map[string]bool{"parser_config": true})
	if err != nil {
		t.Fatalf("UpdateDatasetDocument: code=%v err=%v", code, err)
	}

	var stored entity.Document
	if err := db.Where("id = ?", "doc-1").First(&stored).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	node, ok := stored.ParserConfig["TableChunker:FastFoxesJump"].(map[string]interface{})
	if !ok {
		t.Fatalf("node lost: %v", stored.ParserConfig)
	}
	roles, _ := node["column_roles"].(map[string]interface{})
	if roles["金额"] != "indexing" {
		t.Errorf("role not replaced: %v", node["column_roles"])
	}
	if node["enable_children"] != true {
		t.Errorf("a parameter the patch never mentioned was reset: %v", node)
	}
	if node["column_mode"] != "manual" {
		t.Errorf("column_mode changed without being sent: %v", node["column_mode"])
	}
	tokenizerNode, ok := stored.ParserConfig["Tokenizer:SomeNode"].(map[string]interface{})
	if !ok || tokenizerNode["chunk_token_size"] == nil {
		t.Errorf("another node lost: %v", stored.ParserConfig)
	}
}

func TestUpdateDatasetDocumentColumnPatchRefusesBadValues(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "sales.xlsx", 0, 0)
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").
		Update("parser_config", entity.JSONMap{
			"TableChunker:FastFoxesJump": map[string]interface{}{"column_mode": "auto"},
		}).Error; err != nil {
		t.Fatalf("seed parser_config: %v", err)
	}

	svc := testDocumentService(t)
	_, _, err := svc.UpdateDatasetDocument(t.Context(), "tenant-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{ParserConfig: map[string]interface{}{
			"TableChunker:FastFoxesJump": map[string]interface{}{"column_roles": map[string]interface{}{"金额": "keyword"}},
		}},
		map[string]bool{"parser_config": true})
	if err == nil {
		t.Fatal("an unknown role was accepted")
	}

	var stored entity.Document
	if err := dao.DB.Where("id = ?", "doc-1").First(&stored).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	node := stored.ParserConfig["TableChunker:FastFoxesJump"].(map[string]interface{})
	if node["column_mode"] != "auto" {
		t.Errorf("a refused patch still changed the row: %v", node)
	}
}
