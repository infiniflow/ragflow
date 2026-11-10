package dataset

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type recordingChunkStoreEngine struct {
	fakeChatDocEngine
	calls      int
	baseName   string
	datasetID  string
	vectorSize int
	parserID   string
	language   string
	err        error
	// storeMissing makes ChunkStoreExists report an absent store.
	storeMissing bool
}

func (e *recordingChunkStoreEngine) CreateChunkStore(_ context.Context, baseName, datasetID string, vectorSize int, parserID, language string) error {
	e.calls++
	e.baseName, e.datasetID, e.vectorSize = baseName, datasetID, vectorSize
	e.parserID, e.language = parserID, language
	return e.err
}

func (e *recordingChunkStoreEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return !e.storeMissing, nil
}

// An embedding-model change on an empty dataset must prepare the chunk store for
// the new model. Infinity otherwise keeps only the previous model's
// q_<n>_vec column, so both retrieval and the next parse fail on the new one.
func TestUpdateDatasetPreparesChunkStoreForNewEmbeddingModel(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	eng := &recordingChunkStoreEngine{}
	probes := 0
	svc := testDatasetUpdateService(t)
	svc.docEngine = eng
	svc.embeddingDimension = func(_ context.Context, tenantID, embdID string) (int, error) {
		probes++
		if tenantID != "tenant-1" {
			t.Fatalf("probe tenant = %q, want tenant-1", tenantID)
		}
		if embdID != "BAAI/bge-m3@Builtin" {
			t.Fatalf("probe model = %q, want BAAI/bge-m3@Builtin", embdID)
		}
		return 1024, nil
	}

	model := "BAAI/bge-m3@Builtin"
	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	_, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{EmbeddingModel: &model})
	if err != nil {
		t.Fatalf("UpdateDataset failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if probes != 1 || eng.calls != 1 {
		t.Fatalf("probe calls = %d, chunk store calls = %d, want 1/1", probes, eng.calls)
	}
	if eng.baseName != "ragflow_tenant-1" {
		t.Fatalf("base name = %q, want ragflow_tenant-1", eng.baseName)
	}
	if eng.datasetID != "kb-1" {
		t.Fatalf("dataset id = %q, want kb-1", eng.datasetID)
	}
	if eng.vectorSize != 1024 {
		t.Fatalf("vector size = %d, want 1024", eng.vectorSize)
	}
	if eng.parserID != string(entity.ParserTypeNaive) {
		t.Fatalf("parser id = %q, want %q", eng.parserID, entity.ParserTypeNaive)
	}
}

// A dataset that still holds chunks cannot reinterpret them with a new model, so
// the store must be left alone.
func TestUpdateDatasetKeepsChunkStoreWhenChunksExist(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Update("chunk_num", 3).Error; err != nil {
		t.Fatalf("seed chunk_num: %v", err)
	}

	eng := &recordingChunkStoreEngine{}
	svc := testDatasetUpdateService(t)
	svc.docEngine = eng
	svc.embeddingDimension = func(context.Context, string, string) (int, error) {
		t.Fatal("dimension must not be probed when the dataset still has chunks")
		return 0, nil
	}

	model := "BAAI/bge-m3@Builtin"
	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	if _, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{EmbeddingModel: &model}); err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDataset = (%d, %v), want success", code, err)
	}
	if eng.calls != 0 {
		t.Fatalf("chunk store calls = %d, want 0", eng.calls)
	}
}

// Re-submitting the current model must not touch the store or the provider.
func TestUpdateDatasetSkipsChunkStoreWhenModelUnchanged(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	eng := &recordingChunkStoreEngine{}
	svc := testDatasetUpdateService(t)
	svc.docEngine = eng
	svc.embeddingDimension = func(context.Context, string, string) (int, error) {
		t.Fatal("dimension must not be probed when the model is unchanged")
		return 0, nil
	}

	model := "BAAI/bge-large-zh-v1.5@Builtin"
	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	if _, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{EmbeddingModel: &model}); err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDataset = (%d, %v), want success", code, err)
	}
	if eng.calls != 0 {
		t.Fatalf("chunk store calls = %d, want 0", eng.calls)
	}
}

// A preparation failure must leave the dataset untouched. The store is prepared
// before the commit, so there is no committed model switch to roll back and no
// state in which the retry would skip the repair.
func TestUpdateDatasetLeavesBindingUntouchedWhenChunkStorePreparationFails(t *testing.T) {
	cases := []struct {
		name string
		// failProbe fails the dimension probe instead of the store creation.
		failProbe bool
	}{
		{name: "probe", failProbe: true},
		{name: "create_store", failProbe: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupDatasetUpdateTestDB(t)
			pushServiceDB(t, db)
			insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

			eng := &recordingChunkStoreEngine{}
			if !tc.failProbe {
				eng.err = errors.New("engine unavailable")
			}
			svc := testDatasetUpdateService(t)
			svc.docEngine = eng
			var probeErr error
			if tc.failProbe {
				probeErr = errors.New("provider unavailable")
			}
			svc.embeddingDimension = func(context.Context, string, string) (int, error) {
				return 1024, probeErr
			}

			model := "BAAI/bge-m3@Builtin"
			ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
			_, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{EmbeddingModel: &model})
			if err == nil || code != common.CodeServerError {
				t.Fatalf("UpdateDataset = (%d, %v), want a server error", code, err)
			}
			if !strings.Contains(err.Error(), "prepare the chunk store") {
				t.Fatalf("error %q does not name the chunk store preparation", err)
			}
			if strings.Contains(err.Error(), "dataset settings were saved") {
				t.Fatalf("error %q reports saved settings for an update that changed nothing", err)
			}

			var kb entity.Knowledgebase
			if err := dao.DB.Where("id = ?", "kb-1").Take(&kb).Error; err != nil {
				t.Fatalf("reload dataset: %v", err)
			}
			if kb.EmbdID != "BAAI/bge-large-zh-v1.5@Builtin" {
				t.Fatalf("embd_id after failed preparation = %q, want the previous model", kb.EmbdID)
			}
			if kb.TenantEmbdID != nil {
				t.Fatalf("tenant_embd_id after failed preparation = %q, want nil", *kb.TenantEmbdID)
			}

			eng.err = nil
			probeErr = nil
			svc.embeddingDimension = func(context.Context, string, string) (int, error) {
				return 1024, nil
			}
			if _, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{EmbeddingModel: &model}); err != nil || code != common.CodeSuccess {
				t.Fatalf("retry UpdateDataset = (%d, %v), want success", code, err)
			}
			if err := dao.DB.Where("id = ?", "kb-1").Take(&kb).Error; err != nil {
				t.Fatalf("reload dataset: %v", err)
			}
			if kb.EmbdID != model {
				t.Fatalf("embd_id after retry = %q, want %q", kb.EmbdID, model)
			}
			wantCalls := 2
			if tc.failProbe {
				wantCalls = 1
			}
			if eng.calls != wantCalls {
				t.Fatalf("chunk store calls = %d, want %d", eng.calls, wantCalls)
			}
		})
	}
}

func TestEnsureChunkStoreForEmbeddingWithoutEngine(t *testing.T) {
	svc := &DatasetService{}
	if err := svc.ensureChunkStoreForEmbedding(t.Context(), &entity.Knowledgebase{ID: "kb-1"}, "BAAI/bge-m3@Builtin"); err != nil {
		t.Fatalf("ensureChunkStoreForEmbedding without engine = %v, want nil", err)
	}
}

// A client that disconnects while the store is being prepared must not leave
// the dataset on a model whose store was never prepared: the preparation runs
// before the commit, so the cancelled attempt changes nothing and the retry
// repairs the store.
func TestUpdateDatasetRepairsChunkStoreWhenRequestIsCancelledDuringPreparation(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	eng := &recordingChunkStoreEngine{}
	svc := testDatasetUpdateService(t)
	svc.docEngine = eng

	requestCtx, cancelRequest := context.WithCancel(t.Context())
	defer cancelRequest()
	svc.embeddingDimension = func(context.Context, string, string) (int, error) {
		// The probe runs before the transaction, and the client goes away
		// while it is in flight.
		cancelRequest()
		return 0, context.Canceled
	}

	model := "BAAI/bge-m3@Builtin"
	ctx := withDatasetTestContext(requestCtx, "tenant-1", "tenant-1")
	if _, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{EmbeddingModel: &model}); err == nil || code != common.CodeServerError {
		t.Fatalf("UpdateDataset = (%d, %v), want a server error", code, err)
	}

	var kb entity.Knowledgebase
	if err := dao.DB.Where("id = ?", "kb-1").Take(&kb).Error; err != nil {
		t.Fatalf("reload dataset: %v", err)
	}
	if kb.EmbdID != "BAAI/bge-large-zh-v1.5@Builtin" {
		t.Fatalf("embd_id after a cancelled request = %q, want the previous model", kb.EmbdID)
	}

	svc.embeddingDimension = func(context.Context, string, string) (int, error) { return 1024, nil }
	retryCtx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	if _, code, err := svc.UpdateDataset(retryCtx, "kb-1", service.UpdateDatasetRequest{EmbeddingModel: &model}); err != nil || code != common.CodeSuccess {
		t.Fatalf("retry UpdateDataset = (%d, %v), want success", code, err)
	}
	if eng.calls != 1 {
		t.Fatalf("chunk store calls = %d, want 1 on the retry", eng.calls)
	}
	if err := dao.DB.Where("id = ?", "kb-1").Take(&kb).Error; err != nil {
		t.Fatalf("reload dataset: %v", err)
	}
	if kb.EmbdID != model {
		t.Fatalf("embd_id after retry = %q, want %q", kb.EmbdID, model)
	}
}

// The engine fixes the fulltext analyzer when it creates the store, so a model
// switch that also changes the language must prepare with the language the same
// update persists, not the one it replaces.
func TestUpdateDatasetPreparesChunkStoreWithUpdatedLanguage(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	eng := &recordingChunkStoreEngine{}
	svc := testDatasetUpdateService(t)
	svc.docEngine = eng
	svc.embeddingDimension = func(context.Context, string, string) (int, error) { return 1024, nil }

	model := "BAAI/bge-m3@Builtin"
	language := "Chinese"
	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	if _, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{EmbeddingModel: &model, Language: &language}); err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDataset = (%d, %v), want success", code, err)
	}
	if eng.calls != 1 {
		t.Fatalf("chunk store calls = %d, want 1", eng.calls)
	}
	if eng.language != language {
		t.Fatalf("chunk store language = %q, want %q", eng.language, language)
	}

	var kb entity.Knowledgebase
	if err := dao.DB.Where("id = ?", "kb-1").Take(&kb).Error; err != nil {
		t.Fatalf("reload dataset: %v", err)
	}
	if kb.Language == nil || *kb.Language != language {
		t.Fatalf("persisted language = %v, want %q", kb.Language, language)
	}
}

// A store that was never created has to be created after the commit: creating
// it earlier would bake the new analyzer and dimension into a store the row
// keeps even when the update is rejected.
func TestUpdateDatasetCreatesMissingChunkStoreAfterCommit(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	eng := &recordingChunkStoreEngine{storeMissing: true}
	svc := testDatasetUpdateService(t)
	svc.docEngine = eng
	svc.embeddingDimension = func(context.Context, string, string) (int, error) { return 1024, nil }

	model := "BAAI/bge-m3@Builtin"
	language := "Chinese"
	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	if _, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{EmbeddingModel: &model, Language: &language}); err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDataset = (%d, %v), want success", code, err)
	}
	if eng.calls != 1 {
		t.Fatalf("chunk store calls = %d, want 1", eng.calls)
	}
	if eng.language != language {
		t.Fatalf("chunk store language = %q, want %q", eng.language, language)
	}
}

// A rejected update must leave an absent store absent, so a later request with
// the original settings still sees a clean state.
func TestUpdateDatasetDoesNotCreateChunkStoreWhenUpdateIsRejected(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	insertDatasetUpdateKB(t, "kb-2", "tenant-1", "Existing")

	eng := &recordingChunkStoreEngine{storeMissing: true}
	svc := testDatasetUpdateService(t)
	svc.docEngine = eng
	svc.embeddingDimension = func(context.Context, string, string) (int, error) { return 1024, nil }

	model := "BAAI/bge-m3@Builtin"
	duplicate := "Existing"
	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	_, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{Name: &duplicate, EmbeddingModel: &model})
	if err == nil || code != common.CodeDataError {
		t.Fatalf("UpdateDataset = (%d, %v), want a duplicate-name error", code, err)
	}
	if eng.calls != 0 {
		t.Fatalf("chunk store calls = %d, want 0 for a rejected update", eng.calls)
	}

	var kb entity.Knowledgebase
	if err := dao.DB.Where("id = ?", "kb-1").Take(&kb).Error; err != nil {
		t.Fatalf("reload dataset: %v", err)
	}
	if kb.EmbdID != "BAAI/bge-large-zh-v1.5@Builtin" {
		t.Fatalf("embd_id after a rejected update = %q, want the previous model", kb.EmbdID)
	}

	name := "Renamed"
	if _, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{Name: &name, EmbeddingModel: &model}); err != nil || code != common.CodeSuccess {
		t.Fatalf("retry UpdateDataset = (%d, %v), want success", code, err)
	}
	if eng.calls != 1 {
		t.Fatalf("chunk store calls = %d, want 1 on the retry", eng.calls)
	}
}

// The retry after a failed post-commit creation sends the same model. The store
// is still missing, so the update must create it again instead of skipping the
// preparation because the model did not change.
func TestUpdateDatasetRepairsMissingChunkStoreOnSameModelUpdate(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")

	eng := &recordingChunkStoreEngine{storeMissing: true}
	svc := testDatasetUpdateService(t)
	svc.docEngine = eng
	svc.embeddingDimension = func(context.Context, string, string) (int, error) { return 1024, nil }

	model := "BAAI/bge-large-zh-v1.5@Builtin"
	name := "Renamed"
	ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
	if _, code, err := svc.UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{Name: &name, EmbeddingModel: &model}); err != nil || code != common.CodeSuccess {
		t.Fatalf("UpdateDataset = (%d, %v), want success", code, err)
	}
	if eng.calls != 1 {
		t.Fatalf("chunk store calls = %d, want 1", eng.calls)
	}
	if eng.parserID != string(entity.ParserTypeNaive) {
		t.Fatalf("chunk store parser = %q, want %q", eng.parserID, entity.ParserTypeNaive)
	}
}
