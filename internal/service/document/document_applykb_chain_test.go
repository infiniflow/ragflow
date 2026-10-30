package document

// Real writer-to-reader ApplyKB chain tests (YU-366).
//
// These tests never hand-craft the dataset's parser_config. The dataset side
// always goes through a real writer entry point:
//   - DatasetService.UpdateMetadataConfig (dataset metadata API), or
//   - DatasetService.CreateDataset (dataset creation from a pipeline DSL),
// after which the KB row is reloaded from the database and only then fed to
// DocumentService.Ingest(ApplyKB:true). The document row is reloaded as well
// and asserted through the pipeline's own consumer
// (common.ExtractorMetadataConfig) — dataset service -> DB -> Ingest ->
// DB -> consumer, no shortcuts.

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/service"
	datasetsvc "ragflow/internal/service/dataset"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal parser_config: %v", err)
	}
	return string(data)
}

// firstExtractorMetadata returns the modular metadata of the first (sorted)
// Extractor node carrying one, mirroring the deterministic reader contract.
func firstExtractorMetadata(t *testing.T, parserConfig entity.JSONMap) map[string]any {
	t.Helper()
	var keys []string
	for k := range parserConfig {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "extractor:") || strings.HasPrefix(lower, "extractor_") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		node, ok := parserConfig[k].(map[string]any)
		if !ok {
			continue
		}
		if meta, ok := node["metadata"].(map[string]any); ok {
			return meta
		}
	}
	t.Fatalf("no Extractor node with modular metadata in %#v", parserConfig)
	return nil
}

func extractorNodeLLMID(t *testing.T, parserConfig entity.JSONMap, node string) string {
	t.Helper()
	params, ok := parserConfig[node].(map[string]any)
	if !ok {
		t.Fatalf("node %s missing or not a map: %#v", node, parserConfig[node])
	}
	llmID, _ := params["llm_id"].(string)
	return llmID
}

// newDatasetService builds a DatasetService the same way production code does.
// Only the kb/tenant DAOs are exercised by these tests.
func newDatasetService() *datasetsvc.DatasetService {
	return datasetsvc.NewDatasetService()
}

// migrateChainModelTables adds the tables the real dataset entry points touch
// (model resolution, connectors, canvas) beyond the document package's base
// migration. Test scaffolding only; the dataset writes themselves stay real.
func migrateChainModelTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(
		&entity.Connector{},
		&entity.Connector2Kb{},
		&entity.SyncLogs{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
		&entity.UserCanvas{},
		&entity.PipelineOperationLog{},
	); err != nil {
		t.Fatalf("migrate chain tables: %v", err)
	}
}

// TestIngest_ApplyKB_RealDatasetAPIChain drives the real dataset metadata API
// (UpdateMetadataConfig with enabled=true, a custom author field and a
// built-in field), reloads the KB from the database, then Ingests with
// ApplyKB:true and asserts the persisted document config through the
// pipeline's own consumer.
func TestIngest_ApplyKB_RealDatasetAPIChain(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)
	// Every real dataset DSL defines an Extractor node; the writer under test
	// scopes onto it. Scaffolding the empty node shape is not faking config:
	// no metadata values are invented here.
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Update(
		"parser_config", entity.JSONMap{
			"Extractor:AutoExtractDefault": map[string]any{},
		},
	).Error; err != nil {
		t.Fatalf("seed kb extractor node: %v", err)
	}
	insertTestDoc(t, "doc-1", "kb-1", 9, 4)
	if err := dao.DB.Model(&entity.Document{}).Where("id = ?", "doc-1").Updates(map[string]interface{}{
		"location": "loc-1",
		"parser_config": entity.JSONMap{
			"chunk_token_num": 128,
			"Extractor:AutoExtractDefault": map[string]any{
				"llm_id": "",
			},
		},
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	ctx := documentTestContext(t.Context())
	enabled := true
	if _, code, err := newDatasetService().UpdateMetadataConfig(ctx, "kb-1", "user-1", &service.MetadataConfigRequest{
		Enabled:         &enabled,
		Metadata:        []service.MetadataConfigField{{Key: "author", Type: "string"}},
		BuiltInMetadata: []service.MetadataConfigField{{Key: "document_name", Type: "string"}},
	}); err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateMetadataConfig err=%v code=%v", err, code)
	}

	// Reload the KB: this is the real stored shape, not a fixture.
	kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, "kb-1")
	if err != nil {
		t.Fatalf("reload kb: %v", err)
	}
	t.Logf("stored kb parser_config: %s", mustJSON(t, kb.ParserConfig))
	kbMeta := firstExtractorMetadata(t, kb.ParserConfig)
	if enabled, _ := kbMeta["enabled"].(bool); !enabled {
		t.Fatalf("stored dataset metadata.enabled = %v, want true", kbMeta["enabled"])
	}
	if !metadataFieldKeys(kbMeta)["author"] {
		t.Fatalf("stored dataset metadata missing author: %#v", kbMeta["metadata"])
	}
	kbLLMID := extractorNodeLLMID(t, kb.ParserConfig, "Extractor:AutoExtractDefault")
	if kbLLMID == "" {
		t.Fatalf("stored dataset node llm_id is empty; tenant default was not applied")
	}
	t.Logf("stored dataset node llm_id: %s", kbLLMID)

	publisher := &recordingTaskPublisher{}
	svc := testDocumentService(t)
	svc.ingestionTaskSvc.SetTaskPublisher(publisher)

	code, err := svc.Ingest(ctx, "user-1", &IngestDocumentRequest{
		DocIDs:  []string{"doc-1"},
		Run:     string(entity.TaskStatusRunning),
		Delete:  true,
		ApplyKB: true,
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected code %v, got %v", common.CodeSuccess, code)
	}

	got, err := svc.documentDAO.GetByID(ctx, db, "doc-1")
	if err != nil {
		t.Fatalf("reload doc: %v", err)
	}
	t.Logf("persisted doc parser_config: %s", mustJSON(t, got.ParserConfig))
	auto, ok := got.ParserConfig["Extractor:AutoExtractDefault"].(map[string]any)
	if !ok {
		t.Fatalf("Extractor:AutoExtractDefault missing: %#v", got.ParserConfig)
	}
	if auto["llm_id"] != kbLLMID {
		t.Fatalf("persisted node llm_id = %v, want dataset value %v", auto["llm_id"], kbLLMID)
	}
	meta, ok := auto["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("persisted node metadata missing: %#v", auto)
	}
	if enabled, _ := meta["enabled"].(bool); !enabled {
		t.Fatalf("persisted metadata.enabled = %v, want true", meta["enabled"])
	}
	if !metadataFieldKeys(meta)["author"] {
		t.Fatalf("persisted metadata missing author: %#v", meta["metadata"])
	}
	if _, ok := got.ParserConfig["llm_id"]; ok {
		t.Fatalf("top-level llm_id written: %#v", got.ParserConfig)
	}
	if _, ok := got.ParserConfig["enable_metadata"]; ok {
		t.Fatalf("flat enable_metadata written: %#v", got.ParserConfig)
	}
	if _, ok := got.ParserConfig["chunk_token_num"]; !ok {
		t.Fatalf("chunk_token_num dropped: %#v", got.ParserConfig)
	}
	consumerMeta, ok := common.ExtractorMetadataConfig(map[string]any(got.ParserConfig))
	if !ok {
		t.Fatalf("consumer cannot read persisted extractor metadata: %#v", got.ParserConfig)
	}
	if enabled, _ := consumerMeta["enabled"].(bool); !enabled {
		t.Fatalf("consumer metadata.enabled = %v, want true", consumerMeta["enabled"])
	}
}

// TestIngest_ApplyKB_DisabledDatasetAPIChain pins the real disabled path: a
// dataset explicitly configured with enabled=false must not enable document
// auto-metadata.
func TestIngest_ApplyKB_DisabledDatasetAPIChain(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Update(
		"parser_config", entity.JSONMap{
			"Extractor:AutoExtractDefault": map[string]any{},
		},
	).Error; err != nil {
		t.Fatalf("seed kb extractor node: %v", err)
	}
	insertTestDoc(t, "doc-1", "kb-1", 9, 4)
	if err := dao.DB.Model(&entity.Document{}).Where("id = ?", "doc-1").Updates(map[string]interface{}{
		"location": "loc-1",
		"parser_config": entity.JSONMap{
			"chunk_token_num": 128,
			"Extractor:AutoExtractDefault": map[string]any{
				"llm_id": "",
			},
		},
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	ctx := documentTestContext(t.Context())
	disabled := false
	if _, code, err := newDatasetService().UpdateMetadataConfig(ctx, "kb-1", "user-1", &service.MetadataConfigRequest{
		Enabled: &disabled,
	}); err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateMetadataConfig err=%v code=%v", err, code)
	}

	publisher := &recordingTaskPublisher{}
	svc := testDocumentService(t)
	svc.ingestionTaskSvc.SetTaskPublisher(publisher)

	code, err := svc.Ingest(ctx, "user-1", &IngestDocumentRequest{
		DocIDs:  []string{"doc-1"},
		Run:     string(entity.TaskStatusRunning),
		Delete:  true,
		ApplyKB: true,
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected code %v, got %v", common.CodeSuccess, code)
	}

	got, err := svc.documentDAO.GetByID(ctx, db, "doc-1")
	if err != nil {
		t.Fatalf("reload doc: %v", err)
	}
	auto, ok := got.ParserConfig["Extractor:AutoExtractDefault"].(map[string]any)
	if !ok {
		t.Fatalf("Extractor:AutoExtractDefault missing: %#v", got.ParserConfig)
	}
	meta, ok := auto["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("node metadata missing: %#v", auto)
	}
	if enabled, _ := meta["enabled"].(bool); enabled {
		t.Fatalf("node metadata.enabled = %v, want false", meta["enabled"])
	}
	consumerMeta, ok := common.ExtractorMetadataConfig(map[string]any(got.ParserConfig))
	if !ok {
		t.Fatalf("consumer cannot read node extractor metadata: %#v", got.ParserConfig)
	}
	if enabled, _ := consumerMeta["enabled"].(bool); enabled {
		t.Fatalf("consumer metadata.enabled = %v, want false", consumerMeta["enabled"])
	}
}

// TestIngest_ApplyKB_RealCreatedDatasetChain covers the stored shape where
// the dataset writer persisted component-scoped config with no top-level
// "metadata" key (dataset creation scopes the DSL config before persisting).
// The seed goes through the real DatasetService.CreateDataset entry point.
func TestIngest_ApplyKB_RealCreatedDatasetChain(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")
	migrateChainModelTables(t, db)
	// Mirror the dataset package's own fixture: the membership carries the
	// resolved embedding model id, keeping CreateDataset on its real path
	// (no embedding override).
	if err := dao.DB.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", "user-1", "tenant-1").Update(
		"tenant_embd_id", "embd-1",
	).Error; err != nil {
		t.Fatalf("set membership embd id: %v", err)
	}

	ctx := documentTestContext(t.Context())
	parserID := "general"
	parseType := 1
	team := "team"
	result, code, err := newDatasetService().CreateDataset(ctx, &service.CreateDatasetRequest{
		Name:       "ds-chain",
		ParserID:   &parserID,
		ParseType:  &parseType,
		Permission: &team,
		ParserConfig: map[string]interface{}{
			"Extractor:AutoExtractDefault": map[string]interface{}{
				"llm_id": "",
				"metadata": map[string]interface{}{
					"enabled": true,
					"metadata": []interface{}{
						map[string]interface{}{"key": "author", "type": "string"},
					},
					"built_in_metadata": []interface{}{
						map[string]interface{}{"key": "document_name", "type": "string"},
					},
				},
			},
		},
	}, permission.Subject{UserID: "user-1", TenantID: "tenant-1"})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("CreateDataset err=%v code=%v", err, code)
	}
	kbID, _ := result["id"].(string)
	if kbID == "" {
		t.Fatalf("created dataset has no id: %#v", result)
	}

	// Reload the KB: record the real stored shape the writer produced.
	kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, kbID)
	if err != nil {
		t.Fatalf("reload kb: %v", err)
	}
	t.Logf("stored kb parser_config: %s", mustJSON(t, kb.ParserConfig))
	if _, ok := kb.ParserConfig["metadata"]; ok {
		t.Fatalf("stored kb has top-level metadata, want component-scoped only: %s", mustJSON(t, kb.ParserConfig))
	}
	kbMeta := firstExtractorMetadata(t, kb.ParserConfig)
	if enabled, _ := kbMeta["enabled"].(bool); !enabled {
		t.Fatalf("stored dataset metadata.enabled = %v, want true", kbMeta["enabled"])
	}
	if !metadataFieldKeys(kbMeta)["author"] {
		t.Fatalf("stored dataset metadata missing author: %#v", kbMeta["metadata"])
	}
	kbLLMID := ""
	for _, k := range []string{"Extractor:AutoExtractDefault"} {
		kbLLMID = extractorNodeLLMID(t, kb.ParserConfig, k)
	}
	if kbLLMID == "" {
		t.Fatalf("stored dataset node llm_id is empty; tenant default was not applied")
	}
	t.Logf("stored dataset node llm_id: %s", kbLLMID)

	insertTestDoc(t, "doc-1", kbID, 9, 4)
	if err := dao.DB.Model(&entity.Document{}).Where("id = ?", "doc-1").Updates(map[string]interface{}{
		"location": "loc-1",
		"parser_config": entity.JSONMap{
			"chunk_token_num": 128,
			"Extractor:AutoExtractDefault": map[string]any{
				"llm_id": "",
			},
		},
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	publisher := &recordingTaskPublisher{}
	svc := testDocumentService(t)
	svc.ingestionTaskSvc.SetTaskPublisher(publisher)

	code, err = svc.Ingest(ctx, "user-1", &IngestDocumentRequest{
		DocIDs:  []string{"doc-1"},
		Run:     string(entity.TaskStatusRunning),
		Delete:  true,
		ApplyKB: true,
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected code %v, got %v", common.CodeSuccess, code)
	}

	got, err := svc.documentDAO.GetByID(ctx, db, "doc-1")
	if err != nil {
		t.Fatalf("reload doc: %v", err)
	}
	t.Logf("persisted doc parser_config: %s", mustJSON(t, got.ParserConfig))
	auto, ok := got.ParserConfig["Extractor:AutoExtractDefault"].(map[string]any)
	if !ok {
		t.Fatalf("Extractor:AutoExtractDefault missing: %#v", got.ParserConfig)
	}
	if auto["llm_id"] != kbLLMID {
		t.Fatalf("persisted node llm_id = %v, want dataset value %v", auto["llm_id"], kbLLMID)
	}
	meta, ok := auto["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("persisted node metadata missing: %#v", auto)
	}
	if enabled, _ := meta["enabled"].(bool); !enabled {
		t.Fatalf("persisted metadata.enabled = %v, want true", meta["enabled"])
	}
	if !metadataFieldKeys(meta)["author"] {
		t.Fatalf("persisted metadata missing author: %#v", meta["metadata"])
	}
	if _, ok := got.ParserConfig["llm_id"]; ok {
		t.Fatalf("top-level llm_id written: %#v", got.ParserConfig)
	}
	if _, ok := got.ParserConfig["enable_metadata"]; ok {
		t.Fatalf("flat enable_metadata written: %#v", got.ParserConfig)
	}
	if _, ok := got.ParserConfig["metadata"]; ok {
		t.Fatalf("top-level metadata written: %#v", got.ParserConfig)
	}
	if _, ok := got.ParserConfig["chunk_token_num"]; !ok {
		t.Fatalf("chunk_token_num dropped: %#v", got.ParserConfig)
	}
	consumerMeta, ok := common.ExtractorMetadataConfig(map[string]any(got.ParserConfig))
	if !ok {
		t.Fatalf("consumer cannot read persisted extractor metadata: %#v", got.ParserConfig)
	}
	if enabled, _ := consumerMeta["enabled"].(bool); !enabled {
		t.Fatalf("consumer metadata.enabled = %v, want true", consumerMeta["enabled"])
	}
}
