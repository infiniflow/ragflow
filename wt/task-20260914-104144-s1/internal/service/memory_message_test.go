package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
)

func TestIsMessageDocumentNotFound(t *testing.T) {
	if !isMessageDocumentNotFound(fmt.Errorf("wrapped: %w", enginetypes.ErrDocumentNotFound)) {
		t.Fatal("expected wrapped document-not-found error to be recognized")
	}

	if isMessageDocumentNotFound(errors.New("index does not exist")) {
		t.Fatal("expected unrelated backend error to remain a server error")
	}
}

func TestMemoryIndexNameMatchesPythonPrefix(t *testing.T) {
	t.Setenv(common.EnvESIndexPrefix, "")
	if got := memoryIndexName("tenant-1"); got != "memory_tenant-1" {
		t.Fatalf("memoryIndexName() = %q", got)
	}
	t.Setenv(common.EnvESIndexPrefix, "legacy")
	if got := memoryIndexName("tenant-1"); got != "memory_legacy_tenant-1" {
		t.Fatalf("memoryIndexName() with prefix = %q", got)
	}
}

// The FusionExpr weight slots are [text, vector], and keywords_similarity_weight is the
// text weight, so it belongs in slot 0. Both this and Python's
// api/db/joint_services/memory_message_service.py emitted the pair reversed, which handed
// every memory search the inverse of the requested hybrid balance. The weights below are
// asymmetric on purpose: an even split cannot tell the two orders apart.
func TestMemoryFusionWeightsPutTheKeywordWeightInTheTextSlot(t *testing.T) {
	for _, keywordsSimilarityWeight := range []float64{0.7, 0.9, 0.2} {
		weights := memoryFusionWeights(keywordsSimilarityWeight)
		parts := strings.Split(weights, ",")
		if len(parts) != 2 {
			t.Fatalf("memoryFusionWeights(%v) = %q, want two comma-separated weights", keywordsSimilarityWeight, weights)
		}

		textWeight, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			t.Fatalf("text weight %q: %v", parts[0], err)
		}
		vectorWeight, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			t.Fatalf("vector weight %q: %v", parts[1], err)
		}

		// Compared with a tolerance, not for equality: the slot each weight lands in is
		// the invariant, and %.2f is as valid a rendering here as %.6g.
		if math.Abs(textWeight-keywordsSimilarityWeight) > 1e-9 {
			t.Fatalf("text weight = %v, want %v (from %q)", textWeight, keywordsSimilarityWeight, weights)
		}
		if math.Abs(vectorWeight-(1-keywordsSimilarityWeight)) > 1e-9 {
			t.Fatalf("vector weight = %v, want %v (from %q)", vectorWeight, 1-keywordsSimilarityWeight, weights)
		}
	}
}

// The slot test above deliberately accepts any numeric rendering, so nothing there
// would notice a quiet return to %g. This pins the other half: the strings below are
// what Python's :g emits for the same inputs in
// api/db/joint_services/memory_message_service.py, so a regression that reintroduces
// 0.30000000000000004 on the Go side turns this red. 0.1234567 is here because it is
// the case that actually exercises the six-digit rounding.
func TestMemoryFusionWeightsMatchesPythonFormatting(t *testing.T) {
	for _, testCase := range []struct {
		keywordsSimilarityWeight float64
		want                     string
	}{
		{keywordsSimilarityWeight: 0.7, want: "0.7,0.3"},
		{keywordsSimilarityWeight: 0.9, want: "0.9,0.1"},
		{keywordsSimilarityWeight: 0.1234567, want: "0.123457,0.876543"},
	} {
		if got := memoryFusionWeights(testCase.keywordsSimilarityWeight); got != testCase.want {
			t.Fatalf("memoryFusionWeights(%v) = %q, want %q", testCase.keywordsSimilarityWeight, got, testCase.want)
		}
	}
}

func TestValidateMemorySearchModels(t *testing.T) {
	firstTenantEmbeddingID := "tenant-embedding-1"
	secondTenantEmbeddingID := "tenant-embedding-2"
	if err := validateMemorySearchModels([]*entity.Memory{
		{ID: "memory-1", EmbdID: "embedding@provider", TenantEmbdID: &firstTenantEmbeddingID},
		{ID: "memory-2", EmbdID: "embedding@provider", TenantEmbdID: &secondTenantEmbeddingID},
	}); err != nil {
		t.Fatalf("same memory embedding model rejected: %v", err)
	}

	if err := validateMemorySearchModels([]*entity.Memory{
		{ID: "memory-1", EmbdID: "embedding-a@provider"},
		{ID: "memory-2", EmbdID: "embedding-b@provider"},
	}); err == nil {
		t.Fatal("different memory embedding models were accepted")
	}
}

func TestRequireMemoryAccessReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ctx.Err()
	if _, gotErr := NewMemoryService().requireMemoryAccess(ctx, "user-1", "memory-1"); !errors.Is(gotErr, err) {
		t.Fatalf("requireMemoryAccess error = %v, want %v", gotErr, err)
	}
}

type memoryMessageDocEngine struct {
	fakeChatDocEngine
	engineType  string
	searchReq   *enginetypes.SearchRequest
	searchResp  *enginetypes.SearchResult
	updateCond  map[string]interface{}
	updateValue map[string]interface{}
	updateBase  string
	updateID    string
}

func (e *memoryMessageDocEngine) Search(ctx context.Context, req *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	e.searchReq = req
	if e.searchResp != nil {
		return e.searchResp, nil
	}
	return &enginetypes.SearchResult{}, nil
}

func (e *memoryMessageDocEngine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, baseName string, datasetID string) error {
	e.updateCond = condition
	e.updateValue = newValue
	e.updateBase = baseName
	e.updateID = datasetID
	return nil
}

func (e *memoryMessageDocEngine) FilterDocIdsByMetaPushdown(_ context.Context, _ *gorm.DB, _ []string, _ []map[string]interface{}, _ string) []string {
	return nil
}

func (e *memoryMessageDocEngine) GetType() string {
	if e.engineType != "" {
		return e.engineType
	}
	return "memory"
}

func setupMemoryMessageTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.Memory{},
		&entity.User{},
		&entity.UserTenant{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("failed to migrate memory test tables: %v", err)
	}

	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() {
		dao.DB = orig
	})
}

func TestForgetMessageKeepsCompanionFieldForNonOceanBaseEngines(t *testing.T) {
	setupMemoryMessageTestDB(t)

	// Pin the clock to a fixed instant in a fixed non-UTC location so the
	// server-local assertions below hold on any host, including UTC CI
	// runners.
	pinned := time.Date(2026, 8, 20, 10, 5, 0, 0, time.FixedZone("UTC+8", 8*3600))
	pinMemoryNow(t, pinned)

	if err := dao.DB.Create(&entity.Memory{
		ID:          "memory-1",
		Name:        "Test memory",
		TenantID:    "user-1",
		MemoryType:  dao.MemoryTypeRaw,
		StorageType: "table",
		EmbdID:      "embd",
		LLMID:       "llm",
		Permissions: string(entity.TenantPermissionMe),
		MemorySize:  MemorySizeLimit,
	}).Error; err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	for _, test := range []struct {
		engineType            string
		wantForgetAtCompanion bool
	}{
		{engineType: "elasticsearch", wantForgetAtCompanion: true},
		{engineType: "infinity", wantForgetAtCompanion: true},
		{engineType: "oceanbase", wantForgetAtCompanion: false},
		{engineType: "seekdb", wantForgetAtCompanion: false},
	} {
		t.Run(test.engineType, func(t *testing.T) {
			docEngine := &memoryMessageDocEngine{engineType: test.engineType}
			service := NewMemoryService()
			service.docEngine = docEngine

			if err := service.ForgetMessage(t.Context(), "user-1", "memory-1", 42); err != nil {
				t.Fatalf("ForgetMessage() error = %v", err)
			}
			if docEngine.updateCond["id"] != "memory-1_42" {
				t.Fatalf("update condition = %#v", docEngine.updateCond)
			}
			if docEngine.updateBase != "memory_user-1" || docEngine.updateID != "memory-1" {
				t.Fatalf("update target = (%q, %q)", docEngine.updateBase, docEngine.updateID)
			}
			if forgetAt, ok := docEngine.updateValue["forget_at"].(string); !ok || forgetAt == "" {
				t.Fatalf("forget_at = %#v, want non-empty string", docEngine.updateValue["forget_at"])
			} else if want := "2026-08-20 10:05:00"; forgetAt != want {
				// forget_at must be the server-local wall clock, not a
				// UTC-shifted one (which would be "2026-08-20 02:05:00").
				t.Fatalf("forget_at = %q, want server-local wall clock %q", forgetAt, want)
			}
			companion, hasCompanion := docEngine.updateValue["forget_at_flt"]
			if hasCompanion != test.wantForgetAtCompanion {
				t.Fatalf("forget_at_flt present = %t, want %t", hasCompanion, test.wantForgetAtCompanion)
			}
			if hasCompanion && companion != pinned.UnixMilli() {
				t.Fatalf("forget_at_flt = %v, want Unix milliseconds of the stamped instant %d", companion, pinned.UnixMilli())
			}
		})
	}
}

func TestUpdateMemoryTeamMemberCannotChangePermissions(t *testing.T) {
	setupMemoryMessageTestDB(t)

	status := "1"
	if err := dao.DB.Create(&entity.Memory{
		ID:          "mem-team",
		Name:        "Shared memory",
		TenantID:    "owner-1",
		MemoryType:  dao.MemoryTypeRaw,
		StorageType: "table",
		EmbdID:      "embd",
		LLMID:       "llm",
		Permissions: string(entity.TenantPermissionTeam),
		MemorySize:  MemorySizeLimit,
	}).Error; err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	if err := dao.DB.Create(&entity.UserTenant{
		ID:        "ut-team",
		UserID:    "member-1",
		TenantID:  "owner-1",
		Role:      "normal",
		InvitedBy: "owner-1",
		Status:    &status,
	}).Error; err != nil {
		t.Fatalf("seed user tenant: %v", err)
	}

	svc := NewMemoryService()
	samePermission := " TEAM "
	if _, err := svc.UpdateMemory(t.Context(), "member-1", "mem-team", &UpdateMemoryRequest{
		Description: sptr("member edit"),
		Permissions: &samePermission,
	}); err != nil {
		t.Fatalf("UpdateMemory same permission error = %v", err)
	}

	nextPermission := "me"
	if _, err := svc.UpdateMemory(t.Context(), "member-1", "mem-team", &UpdateMemoryRequest{
		Permissions: &nextPermission,
	}); err == nil {
		t.Fatal("UpdateMemory permission change error = nil, want error")
	}
}

func TestUpdateMemoryTeamMemberResolvesModelsAgainstOwnerTenant(t *testing.T) {
	setupMemoryMessageTestDB(t)

	status := "1"
	if err := dao.DB.Create(&entity.Memory{
		ID:          "mem-model",
		Name:        "Shared model memory",
		TenantID:    "owner-1",
		MemoryType:  dao.MemoryTypeRaw,
		StorageType: "table",
		EmbdID:      "old-embd",
		LLMID:       "old-llm",
		Permissions: string(entity.TenantPermissionTeam),
		MemorySize:  MemorySizeLimit,
	}).Error; err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	if err := dao.DB.Create(&entity.UserTenant{
		ID:        "ut-model",
		UserID:    "member-1",
		TenantID:  "owner-1",
		Role:      "normal",
		InvitedBy: "owner-1",
		Status:    &status,
	}).Error; err != nil {
		t.Fatalf("seed user tenant: %v", err)
	}

	for _, row := range []struct {
		providerID string
		tenantID   string
		modelID    string
	}{
		{providerID: "provider-owner", tenantID: "owner-1", modelID: "tenant-llm-owner"},
		{providerID: "provider-member", tenantID: "member-1", modelID: "tenant-llm-member"},
	} {
		if err := dao.DB.Create(&entity.TenantModelProvider{
			ID:           row.providerID,
			ProviderName: "OpenAI",
			TenantID:     row.tenantID,
		}).Error; err != nil {
			t.Fatalf("seed provider %s: %v", row.providerID, err)
		}
		instanceID := row.providerID + "-default"
		if err := dao.DB.Create(&entity.TenantModelInstance{
			ID:           instanceID,
			InstanceName: "default",
			ProviderID:   row.providerID,
			APIKey:       "test-key",
			Status:       "active",
		}).Error; err != nil {
			t.Fatalf("seed instance %s: %v", instanceID, err)
		}
		if err := dao.DB.Create(&entity.TenantModel{
			ID:         row.modelID,
			ModelName:  "gpt-4o",
			ProviderID: row.providerID,
			InstanceID: instanceID,
			ModelType:  int(entity.ModelTypeChat),
			Status:     "active",
		}).Error; err != nil {
			t.Fatalf("seed model %s: %v", row.modelID, err)
		}
	}

	llmID := "gpt-4o@default@OpenAI"
	if _, err := NewMemoryService().UpdateMemory(t.Context(), "member-1", "mem-model", &UpdateMemoryRequest{
		LLMID: &llmID,
	}); err != nil {
		t.Fatalf("UpdateMemory model error = %v", err)
	}

	updated, err := dao.NewMemoryDAO().GetByID(t.Context(), dao.DB, "mem-model")
	if err != nil {
		t.Fatalf("get updated memory: %v", err)
	}
	if updated.TenantLLMID == nil || *updated.TenantLLMID != "tenant-llm-owner" {
		t.Fatalf("tenant_llm_id = %v, want tenant-llm-owner", updated.TenantLLMID)
	}
}

func TestListMemoriesUsesTenantModelIDForDisplayName(t *testing.T) {
	setupMemoryMessageTestDB(t)

	if err := dao.DB.Create(&entity.User{ID: "user-1", Nickname: "Owner"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := dao.DB.Create(&entity.TenantModelProvider{
		ID:           "provider-1",
		ProviderName: "OpenAI",
		TenantID:     "user-1",
	}).Error; err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	if err := dao.DB.Create(&entity.TenantModelInstance{
		ID:           "instance-1",
		InstanceName: "default",
		ProviderID:   "provider-1",
		APIKey:       "test-key",
	}).Error; err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	if err := dao.DB.Create(&entity.TenantModel{
		ID:         "tenant-llm-1",
		ModelName:  "gpt-4o",
		ProviderID: "provider-1",
		InstanceID: "instance-1",
		ModelType:  int(entity.ModelTypeChat),
		Status:     "active",
	}).Error; err != nil {
		t.Fatalf("seed chat model: %v", err)
	}
	if err := dao.DB.Create(&entity.TenantModel{
		ID:         "tenant-embd-1",
		ModelName:  "text-embedding-3-small",
		ProviderID: "provider-1",
		InstanceID: "instance-1",
		ModelType:  int(entity.ModelTypeEmbedding),
		Status:     "active",
	}).Error; err != nil {
		t.Fatalf("seed embedding model: %v", err)
	}

	tenantLLMID := "tenant-llm-1"
	tenantEmbdID := "tenant-embd-1"
	if err := dao.DB.Create(&entity.Memory{
		ID:               "mem-with-tenant-models",
		Name:             "With tenant models",
		TenantID:         "user-1",
		MemoryType:       dao.MemoryTypeRaw,
		StorageType:      "table",
		LLMID:            "gpt-4o@OpenAI",
		TenantLLMID:      &tenantLLMID,
		EmbdID:           "text-embedding-3-small@OpenAI",
		TenantEmbdID:     &tenantEmbdID,
		Permissions:      string(TenantPermissionMe),
		ForgettingPolicy: string(ForgettingPolicyFIFO),
	}).Error; err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	ctx := t.Context()
	resp, err := NewMemoryService().ListMemories(ctx, "user-1", []string{"user-1"}, nil, "", "", 1, 10)
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if resp.TotalCount != 1 || len(resp.MemoryList) != 1 {
		t.Fatalf("ListMemories returned total=%d len=%d, want 1", resp.TotalCount, len(resp.MemoryList))
	}
	memory := resp.MemoryList[0]
	if got, want := memory["llm_id"], "gpt-4o@default@OpenAI"; got != want {
		t.Fatalf("llm_id = %v, want %v", got, want)
	}
	if got, want := memory["embd_id"], "text-embedding-3-small@default@OpenAI"; got != want {
		t.Fatalf("embd_id = %v, want %v", got, want)
	}
}

func TestListMemoriesFallsBackToRawModelIDWithoutTenantModelID(t *testing.T) {
	setupMemoryMessageTestDB(t)

	if err := dao.DB.Create(&entity.User{ID: "user-1", Nickname: "Owner"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := dao.DB.Create(&entity.Memory{
		ID:               "mem-without-tenant-models",
		Name:             "Without tenant models",
		TenantID:         "user-1",
		MemoryType:       dao.MemoryTypeRaw,
		StorageType:      "table",
		LLMID:            "raw-llm",
		EmbdID:           "raw-embd",
		Permissions:      string(TenantPermissionMe),
		ForgettingPolicy: string(ForgettingPolicyFIFO),
	}).Error; err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	ctx := t.Context()
	resp, err := NewMemoryService().ListMemories(ctx, "user-1", []string{"user-1"}, nil, "", "", 1, 10)
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if resp.TotalCount != 1 || len(resp.MemoryList) != 1 {
		t.Fatalf("ListMemories returned total=%d len=%d, want 1", resp.TotalCount, len(resp.MemoryList))
	}
	memory := resp.MemoryList[0]
	if got, want := memory["llm_id"], "raw-llm"; got != want {
		t.Fatalf("llm_id = %v, want %v", got, want)
	}
	if got, want := memory["embd_id"], "raw-embd"; got != want {
		t.Fatalf("embd_id = %v, want %v", got, want)
	}
}

func seedMemoryMessages(t *testing.T) {
	t.Helper()

	memories := []*entity.Memory{
		{
			ID:               "mem-owned",
			Name:             "Owned",
			TenantID:         "user-1",
			MemoryType:       dao.MemoryTypeRaw,
			StorageType:      "table",
			EmbdID:           "embd-1",
			LLMID:            "llm-1",
			Permissions:      string(TenantPermissionMe),
			ForgettingPolicy: string(ForgettingPolicyFIFO),
		},
		{
			ID:               "mem-other",
			Name:             "Other",
			TenantID:         "user-2",
			MemoryType:       dao.MemoryTypeRaw,
			StorageType:      "table",
			EmbdID:           "embd-2",
			LLMID:            "llm-2",
			Permissions:      string(TenantPermissionMe),
			ForgettingPolicy: string(ForgettingPolicyFIFO),
		},
	}
	for _, memory := range memories {
		if err := dao.DB.Create(memory).Error; err != nil {
			t.Fatalf("seed memory %s: %v", memory.ID, err)
		}
	}
}

func TestSaveAgentMessageBypassesRequestAccessFilter(t *testing.T) {
	setupMemoryMessageTestDB(t)

	if err := dao.DB.Create(&entity.Memory{
		ID:               "mem-owned",
		Name:             "Owned",
		TenantID:         "user-1",
		MemoryType:       dao.MemoryTypeRaw,
		StorageType:      "table",
		EmbdID:           "embd-1",
		LLMID:            "llm-1",
		Permissions:      string(TenantPermissionMe),
		ForgettingPolicy: string(ForgettingPolicyFIFO),
	}).Error; err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	svc := &MemoryService{memoryDAO: dao.NewMemoryDAO()}
	msg := MemoryMessage{
		AgentID:       "agent-1",
		SessionID:     "session-1",
		UserInput:     "hi",
		AgentResponse: "hello",
	}

	ok, detail, err := svc.AddMessage(t.Context(), "", []string{"mem-owned"}, msg)
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	if ok || detail != "Memory not found." {
		t.Fatalf("AddMessage with empty current user = (%v, %q), want permission-filtered not found", ok, detail)
	}

	ok, detail, err = svc.saveAgentMessage(t.Context(), []string{"mem-owned"}, msg)
	if err != nil {
		t.Fatalf("saveAgentMessage: %v", err)
	}
	if ok {
		t.Fatal("saveAgentMessage unexpectedly succeeded without a message store")
	}
	if strings.Contains(detail, "Memory not found") {
		t.Fatalf("saveAgentMessage was filtered by request user: %q", detail)
	}
	if !strings.Contains(detail, "message store is not initialized") {
		t.Fatalf("saveAgentMessage detail = %q, want message-store failure after memory lookup", detail)
	}
}

func TestGetMessagesFiltersAccessibleMemoryAndBuildsRecentSearch(t *testing.T) {
	setupMemoryMessageTestDB(t)
	seedMemoryMessages(t)

	docEngine := &memoryMessageDocEngine{
		searchResp: &enginetypes.SearchResult{
			Total: 1,
			Chunks: []map[string]interface{}{
				{
					"message_id":   int64(12),
					"message_type": "raw",
					"memory_id":    "mem-owned",
					"user_id":      "user-1",
					"agent_id":     "agent-1",
					"session_id":   "session-1",
					"valid_at":     float64(123),
					"status":       1,
					"content":      "hello",
					"extra":        "should be dropped",
				},
			},
		},
	}
	svc := &MemoryService{memoryDAO: dao.NewMemoryDAO(), docEngine: docEngine}

	got, code, err := svc.GetMessages(t.Context(), []string{"mem-owned", "mem-other"}, "user-1", "agent-1", "session-1", 3)
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code = %v, want %v", code, common.CodeSuccess)
	}
	if len(got) != 1 || got[0]["content"] != "hello" {
		t.Fatalf("unexpected messages: %+v", got)
	}
	if _, ok := got[0]["extra"]; ok {
		t.Fatalf("unexpected non-selected field in response: %+v", got[0])
	}

	req := docEngine.searchReq
	if req == nil {
		t.Fatal("expected doc engine search request")
	}
	if !reflect.DeepEqual(req.IndexNames, []string{"memory_user-1"}) {
		t.Fatalf("IndexNames = %v, want [memory_user-1]", req.IndexNames)
	}
	if len(req.KbIDs) != 0 {
		t.Fatalf("KbIDs = %v, want empty for memory message search", req.KbIDs)
	}
	if !reflect.DeepEqual(req.Filter["memory_id"], []string{"mem-owned"}) {
		t.Fatalf("memory_id filter = %v, want [mem-owned]", req.Filter["memory_id"])
	}
	if req.Filter["agent_id"] != "agent-1" || req.Filter["session_id"] != "session-1" {
		t.Fatalf("unexpected filter: %+v", req.Filter)
	}
	if req.Limit != 3 {
		t.Fatalf("Limit = %d, want 3", req.Limit)
	}
	if req.OrderBy == nil || len(req.OrderBy.Fields) != 1 || req.OrderBy.Fields[0].Field != "valid_at" || req.OrderBy.Fields[0].Type != enginetypes.SortDesc {
		t.Fatalf("unexpected order by: %+v", req.OrderBy)
	}
}

func TestSearchMessageFiltersAccessibleMemoryAndDefaultsStatus(t *testing.T) {
	setupMemoryMessageTestDB(t)
	seedMemoryMessages(t)

	docEngine := &memoryMessageDocEngine{
		searchResp: &enginetypes.SearchResult{
			Total: 1,
			Chunks: []map[string]interface{}{
				{
					"message_id":   int64(13),
					"message_type": "raw",
					"memory_id":    "mem-owned",
					"user_id":      "user-1",
					"agent_id":     "agent-1",
					"session_id":   "session-1",
					"valid_at":     int64(456),
					"status":       1,
					"content":      "matched",
				},
			},
		},
	}
	svc := &MemoryService{memoryDAO: dao.NewMemoryDAO(), docEngine: docEngine}
	filter := map[string]interface{}{
		"memory_id":  []string{"mem-owned", "mem-other"},
		"agent_id":   "agent-1",
		"session_id": "session-1",
		"user_id":    "user-1",
	}
	params := map[string]interface{}{
		"query":                      "",
		"similarity_threshold":       0.2,
		"keywords_similarity_weight": 0.7,
		"top_n":                      5,
	}

	got, code, err := svc.SearchMessage(t.Context(), "user-1", filter, params)
	if err != nil {
		t.Fatalf("SearchMessage error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code = %v, want %v", code, common.CodeSuccess)
	}
	if len(got) != 1 || got[0]["content"] != "matched" {
		t.Fatalf("unexpected search result: %+v", got)
	}

	req := docEngine.searchReq
	if req == nil {
		t.Fatal("expected doc engine search request")
	}
	if !reflect.DeepEqual(req.Filter["memory_id"], []string{"mem-owned"}) {
		t.Fatalf("memory_id filter = %v, want [mem-owned]", req.Filter["memory_id"])
	}
	if req.Filter["status"] != 1 {
		t.Fatalf("status filter = %v, want 1", req.Filter["status"])
	}
	if req.Filter["agent_id"] != "agent-1" || req.Filter["session_id"] != "session-1" || req.Filter["user_id"] != "user-1" {
		t.Fatalf("unexpected filter: %+v", req.Filter)
	}
	if len(req.MatchExprs) != 0 {
		t.Fatalf("empty query should not build match expressions, got %+v", req.MatchExprs)
	}
	if req.Limit != 5 {
		t.Fatalf("Limit = %d, want 5", req.Limit)
	}
}

func TestUpdateMessageUpdatesStatusByMessageDocID(t *testing.T) {
	setupMemoryMessageTestDB(t)
	seedMemoryMessages(t)

	docEngine := &memoryMessageDocEngine{}
	svc := &MemoryService{memoryDAO: dao.NewMemoryDAO(), docEngine: docEngine}

	ok, err := svc.UpdateMessage(t.Context(), "user-1", "mem-owned", 42, true)
	if err != nil {
		t.Fatalf("UpdateMessage error: %v", err)
	}
	if !ok {
		t.Fatal("UpdateMessage returned false")
	}
	if docEngine.updateBase != "memory_user-1" {
		t.Fatalf("baseName = %q, want memory_user-1", docEngine.updateBase)
	}
	if docEngine.updateID != "mem-owned" {
		t.Fatalf("datasetID = %q, want mem-owned", docEngine.updateID)
	}
	if docEngine.updateCond["id"] != "mem-owned_42" {
		t.Fatalf("condition = %+v, want id mem-owned_42", docEngine.updateCond)
	}
	if docEngine.updateValue["status"] != 1 {
		t.Fatalf("status update = %+v, want status 1", docEngine.updateValue)
	}
}
