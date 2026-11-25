package dataset

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service"

	enginetypes "ragflow/internal/engine/types"

	"go.uber.org/zap"
)

// embeddingCheckSample is one sampled chunk with its stored vector, used by the
// embedding availability check.
type embeddingCheckSample struct {
	ChunkID           string
	KbID              string
	DocID             string
	DocName           string
	VectorField       string
	Vector            []float64
	PageNum           interface{}
	Position          interface{}
	Top               interface{}
	ContentWithWeight string
	QuestionKeywords  []string
}

// errEmbeddingCheckEngineSide marks a failure of the document engine or the
// sampled chunk data during CheckEmbedding: it says nothing about the
// embedding model's health, so the gp- failover must not flip a member for it.
var errEmbeddingCheckEngineSide = errors.New("embedding check: engine-side failure")

// CheckEmbedding verifies that a candidate embedding model is compatible with a
// dataset's existing vectors (the standard "switch embedding model" validation).
// It is independent of the retired RunIndex/graph_rag_queue scheduling path.
func (d *DatasetService) CheckEmbedding(ctx context.Context, datasetID string, req *service.CheckEmbeddingRequest) (*service.EmbeddingCheckResponse, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`lack of "Dataset ID"`)
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID}, permission.OperationUse); err != nil {
		code, permissionErr := permissionresponse.Normalize(err)
		return nil, code, permissionErr
	}

	kb, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("invalid Dataset ID")
		}
		return nil, common.CodeServerError, fmt.Errorf("failed to get dataset %s: %w", datasetID, err)
	}

	if req == nil || strings.TrimSpace(req.EmbeddingID) == "" {
		return nil, common.CodeDataError, errors.New("`embd_id` is required")
	}
	embeddingID := strings.TrimSpace(req.EmbeddingID)
	if d.docEngine == nil {
		return nil, common.CodeServerError, errors.New("doc engine not initialized")
	}

	checkNum := defaultEmbeddingCheckNum
	if req.CheckNum != nil {
		checkNum = *req.CheckNum
	}
	if checkNum <= 0 {
		checkNum = defaultEmbeddingCheckNum
	}

	samples, err := d.sampleRandomChunksWithVectors(ctx, kb.TenantID, datasetID, checkNum)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if len(samples) == 0 {
		return &service.EmbeddingCheckResponse{
			Summary: datasetEmbeddingCheckSummary(datasetID, embeddingID, 0, nil, ""),
			Results: nil,
		}, common.CodeSuccess, nil
	}

	// runCheck samples stored chunks and encodes them with the resolved
	// model. Only engine/encode failures are invocation failures; the
	// dimension mismatch below is a client-side verdict and never retries.
	type embeddingCheckRun struct {
		results            []service.EmbeddingCheckResult
		sims               []float64
		sawTitleAndContent bool
	}
	runCheck := func(ctx context.Context, embeddingModel *modelModule.EmbeddingModel) (embeddingCheckRun, common.ErrorCode, error) {
		run := embeddingCheckRun{
			results:            make([]service.EmbeddingCheckResult, 0, len(samples)),
			sims:               make([]float64, 0, len(samples)),
			sawTitleAndContent: false,
		}
		for _, sample := range samples {
			if sample.Vector == nil || len(sample.Vector) == 0 {
				continue
			}

			rawChunk, err := d.docEngine.GetChunk(ctx, fmt.Sprintf("ragflow_%s", kb.TenantID), sample.ChunkID, []string{datasetID})
			if err != nil {
				return run, common.CodeServerError, fmt.Errorf("failed to get sampled chunk %s: %w (%w)", sample.ChunkID, err, errEmbeddingCheckEngineSide)
			}
			chunkMap := datasetMap(rawChunk)
			if len(chunkMap) == 0 {
				return run, common.CodeServerError, fmt.Errorf("sampled chunk %s was not found (%w)", sample.ChunkID, errEmbeddingCheckEngineSide)
			}

			title := datasetString(chunkMap["title_tks"])
			content := datasetString(chunkMap["content_ltks"])

			var titleVector [][]float64
			if title != "" {
				titleVector, err = datasetEncodeEmbedding(ctx, embeddingModel, []string{title})
				if err != nil {
					return run, common.CodeServerError, err
				}
			}
			var contentVector [][]float64
			if content != "" {
				contentVector, err = datasetEncodeEmbedding(ctx, embeddingModel, []string{content})
				if err != nil {
					return run, common.CodeServerError, err
				}
			}

			var vectors [][]float64
			if len(titleVector) > 0 && len(contentVector) > 0 {
				vectors = [][]float64{titleVector[0], contentVector[0]}
				run.sawTitleAndContent = true
			} else if len(titleVector) > 0 {
				vectors = titleVector
			} else if len(contentVector) > 0 {
				vectors = contentVector
			} else {
				continue
			}

			if len(vectors[0]) != len(sample.Vector) {
				return run, common.CodeDataError, fmt.Errorf("Embedding failure. The dimension (%d) of given embedding model is different from the original (%d)", len(vectors[0]), len(sample.Vector))
			}

			var sim float64
			if len(vectors) == 2 {
				simContent := datasetCosSim(vectors[1], sample.Vector)
				simMix := datasetCosSim(datasetMixVectors(vectors[0], vectors[1], 0.1), sample.Vector)
				sim = simContent
				if simMix > sim {
					sim = simMix
					run.sawTitleAndContent = true
				}
			} else {
				sim = datasetCosSim(vectors[0], sample.Vector)
			}
			sim = datasetRoundFloat(sim, 6)

			run.sims = append(run.sims, sim)
			run.results = append(run.results, service.EmbeddingCheckResult{
				ChunkID:     sample.ChunkID,
				DocID:       sample.DocID,
				DocName:     sample.DocName,
				VectorField: sample.VectorField,
				VectorDim:   len(sample.Vector),
				CosSim:      sim,
			})
		}
		return run, common.CodeSuccess, nil
	}

	// Group failover: a gp- reference whose member fails the check flips
	// inactive and the whole check replays once on the next member.
	var (
		run      embeddingCheckRun
		lastCode common.ErrorCode
	)
	err = dao.InvokeWithGroupFailover(ctx, dao.DB, kb.TenantID, entity.ModelTypeEmbedding, embeddingID,
		func(ctx context.Context) (*modelModule.EmbeddingModel, error) {
			return service.NewModelFactory().NewEmbeddingModel(permission.WithTenantID(ctx, kb.TenantID), embeddingID)
		}, nil,
		func(ctx context.Context, embeddingModel *modelModule.EmbeddingModel) error {
			r, code, invErr := runCheck(ctx, embeddingModel)
			lastCode = code
			if invErr != nil {
				return invErr
			}
			run = r
			return nil
		},
		func(err error) bool {
			// Flip only on embedding-provider failures: engine-side failures
			// (document engine outage, missing sampled chunk) say nothing
			// about the model member's health.
			return lastCode == common.CodeServerError && !errors.Is(err, errEmbeddingCheckEngineSide)
		})
	if err != nil {
		if lastCode == 0 {
			// Resolution failed — same mapping as before failover existed.
			return nil, common.CodeDataError, err
		}
		return nil, lastCode, err
	}

	// Aggregate the batch mode explicitly: title_and_content when any sample was
	// matched against the title+content mix, content_only otherwise.
	matchMode := "content_only"
	if run.sawTitleAndContent {
		matchMode = "title_and_content"
	}
	summary := datasetEmbeddingCheckSummary(datasetID, embeddingID, len(samples), run.sims, matchMode)
	response := &service.EmbeddingCheckResponse{Summary: summary, Results: run.results}
	if len(run.sims) == 0 {
		return nil, common.CodeDataError, errors.New("No embedded chunks are available to compare.")
	}
	if summary.AvgCosSim >= 0.9 {
		return response, common.CodeSuccess, nil
	}
	return response, common.CodeNotEffective, errors.New("Embedding model switch failed: the average similarity between old and new vectors is below 0.9, indicating incompatible vector spaces.")
}

// embeddingSwitchProbeText is the input the switch check encodes to learn the
// candidate model's dimension. Only a non-empty string is required: the vector
// length is a property of the model, not of the text.
const embeddingSwitchProbeText = "dimension probe"

// embeddingDimensionProbe measures the vector a candidate model produces. It is
// a package variable so a test can answer without reaching a provider; the
// production implementation resolves the tenant's model the same way
// CheckEmbedding does.
var embeddingDimensionProbe = probeEmbeddingDimension

func probeEmbeddingDimension(ctx context.Context, tenantID, embeddingID string) (int, error) {
	// Group failover: a gp- reference whose member fails the probe call
	// flips inactive and retries once on the next member.
	var dimension int
	err := dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeEmbedding, embeddingID,
		func(ctx context.Context) (*modelModule.EmbeddingModel, error) {
			return service.NewModelFactory().NewEmbeddingModel(permission.WithTenantID(ctx, tenantID), embeddingID)
		}, nil,
		func(ctx context.Context, embeddingModel *modelModule.EmbeddingModel) error {
			vectors, invErr := datasetEncodeEmbedding(ctx, embeddingModel, []string{embeddingSwitchProbeText})
			if invErr != nil {
				return invErr
			}
			if len(vectors) == 0 || len(vectors[0]) == 0 {
				return fmt.Errorf("embedding model %q returned no vector", embeddingID)
			}
			dimension = len(vectors[0])
			return nil
		}, nil)
	if err != nil {
		return 0, err
	}
	return dimension, nil
}

// embeddingSwitchNeedsCheck reports whether a request replaces the embedding
// model of a dataset that already holds chunks, i.e. whether a switch could
// strand stored vectors.
func embeddingSwitchNeedsCheck(kb *entity.Knowledgebase, candidateID string) bool {
	return kb != nil && kb.ChunkNum > 0 && candidateID != "" && candidateID != kb.EmbdID
}

// validateEmbeddingSwitch rejects a model switch that would strand the vectors
// a dataset already stores.
//
// The chunk vector is keyed by its dimension (q_<dim>_vec), so switching to a
// model with a different dimension does not migrate anything: later writes land
// in a column of their own while every existing chunk stays reachable only
// through the old one, and retrieval — which builds the column name from the
// current model — silently stops matching them. The console already asks the
// user to run CheckEmbedding before switching; this repeats the dimension half
// of that check inside the write path, where an API client cannot skip it.
//
// Only the dimension is compared here: the sample costs one engine round trip
// and one provider call, which is affordable on every model change, while the
// similarity half costs tens of provider calls and stays an explicit action.
//
// The guard is skipped, deliberately, when there is nothing at risk:
//   - the dataset holds no chunk yet (its vectors do not exist);
//   - the request repeats the model the dataset already uses;
//   - no sampled chunk carries a vector, so no stored dimension can be lost.
func (d *DatasetService) validateEmbeddingSwitch(ctx context.Context, kb *entity.Knowledgebase, candidateID string) (common.ErrorCode, error) {
	if kb == nil || kb.ChunkNum <= 0 {
		return common.CodeSuccess, nil
	}
	if candidateID == "" || candidateID == kb.EmbdID {
		return common.CodeSuccess, nil
	}
	if d.docEngine == nil {
		// Nothing to check against. The switch itself still succeeds; a
		// deployment without a document engine holds no chunk vectors.
		common.Warn("Dataset update: skipping the embedding dimension check, the document engine is not initialized",
			zap.String("dataset_id", kb.ID))
		return common.CodeSuccess, nil
	}

	storedDim, storedField, err := d.storedVectorDimension(ctx, kb.TenantID, kb.ID)
	if err != nil {
		// Fail closed: an unreadable sample means the switch cannot be proven
		// safe, and letting it through is exactly the silent breakage this
		// check exists to stop.
		return common.CodeServerError, fmt.Errorf("failed to read a stored chunk vector of dataset %s: %w", kb.ID, err)
	}
	if storedDim == 0 {
		return common.CodeSuccess, nil
	}

	candidateDim, err := embeddingDimensionProbe(ctx, kb.TenantID, candidateID)
	if err != nil {
		return common.CodeDataError, err
	}
	if candidateDim == storedDim {
		return common.CodeSuccess, nil
	}
	return common.CodeDataError, fmt.Errorf(
		"embedding model switch rejected: the dataset stores %d-dimensional vectors in %s from %s, "+
			"but the candidate model %s produces %d-dimensional vectors. The existing chunks would no longer be "+
			"reachable by vector search; rebuild the dataset's vectors with the new model instead of switching in place",
		storedDim, storedField, kb.EmbdID, candidateID, candidateDim,
	)
}

// storedVectorDimension returns the dimension of the vector a stored chunk of
// the dataset carries, together with the column it lives in. A dimension of 0
// means none of the sampled chunks holds a vector.
//
// The sample covers several rows because not every chunk carries a vector: a
// chunk the pipeline skipped embedding for would otherwise let the check pass
// while its neighbours still hold vectors in the old space.
//
// A chunk can carry several vector columns once a model switch left an empty
// column next to a populated one. Every column is inspected before the row
// counts as vectorless, because Go does not fix map iteration order and an
// empty column must not hide its neighbour. The answer names the first
// populated column: such columns can only disagree after a switch this check
// let through, and any of them still tells whether the candidate lands in the
// space the dataset is indexed in.
func (d *DatasetService) storedVectorDimension(ctx context.Context, tenantID, datasetID string) (int, string, error) {
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	// Bounded so one switch costs a handful of reads at most.
	const sampleRows = 8
	found, err := d.docEngine.Search(ctx, &enginetypes.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		Offset:     0,
		Limit:      sampleRows,
		Filter: map[string]interface{}{
			"kb_id":         datasetID,
			"available_int": 1,
		},
	})
	if err != nil {
		return 0, "", fmt.Errorf("failed to sample a chunk: %w", err)
	}
	if found == nil || len(found.Chunks) == 0 {
		return 0, "", nil
	}

	// The search returns the projected fields only, so the vector column is
	// read from the chunk itself.
	for _, row := range found.Chunks {
		chunkID := datasetChunkID(row)
		if chunkID == "" {
			continue
		}
		raw, err := d.docEngine.GetChunk(ctx, indexName, chunkID, []string{datasetID})
		if err != nil {
			return 0, "", fmt.Errorf("failed to load chunk %s: %w", chunkID, err)
		}
		chunk := datasetMap(raw)
		if dim, field := datasetFirstStoredVector(chunk); dim > 0 {
			return dim, field, nil
		}
	}
	return 0, "", nil
}

func (d *DatasetService) sampleRandomChunksWithVectors(ctx context.Context, tenantID, datasetID string, n int) ([]embeddingCheckSample, error) {
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	totalResult, err := d.docEngine.Search(ctx, &enginetypes.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		Offset:     0,
		Limit:      1,
		Filter: map[string]interface{}{
			"kb_id":         datasetID,
			"available_int": 1,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to count chunks for dataset %s: %w", datasetID, err)
	}
	if totalResult == nil || totalResult.Total <= 0 {
		return []embeddingCheckSample{}, nil
	}

	total := int(totalResult.Total)
	// Each sampled offset costs an engine Search + GetChunk plus up to two
	// provider calls, so bound the client-controlled sample count server-side.
	const maxEmbeddingSamples = 32
	if n < 0 {
		return nil, fmt.Errorf("invalid sample size: %d", n)
	}
	if n > maxEmbeddingSamples {
		n = maxEmbeddingSamples
	}
	if n > total {
		n = total
	}
	limit := total
	if limit > 1000 {
		limit = 1000
	}
	if n > limit {
		n = limit
	}
	offsets := rand.Perm(limit)
	offsets = offsets[:n]
	sort.Ints(offsets)

	baseFields := []string{"docnm_kwd", "doc_id", "content_with_weight", "page_num_int", "position_int", "top_int"}
	samples := make([]embeddingCheckSample, 0, n)
	var lastSampleErr error
	for _, offset := range offsets {
		searchResult, err := d.docEngine.Search(ctx, &enginetypes.SearchRequest{
			IndexNames:   []string{indexName},
			KbIDs:        []string{datasetID},
			Offset:       offset,
			Limit:        1,
			SelectFields: baseFields,
			Filter: map[string]interface{}{
				"kb_id":         datasetID,
				"available_int": 1,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to sample chunk at offset %d for dataset %s: %w", offset, datasetID, err)
		}
		if searchResult == nil || len(searchResult.Chunks) == 0 {
			lastSampleErr = fmt.Errorf("chunk search returned no result at offset %d for dataset %s", offset, datasetID)
			continue
		}
		chunkID := datasetChunkID(searchResult.Chunks[0])
		if chunkID == "" {
			lastSampleErr = fmt.Errorf("sampled chunk at offset %d for dataset %s is missing an ID", offset, datasetID)
			continue
		}
		fullChunk, err := d.docEngine.GetChunk(ctx, indexName, chunkID, []string{datasetID})
		if err != nil {
			return nil, fmt.Errorf("failed to get sampled chunk %s: %w", chunkID, err)
		}
		chunkMap := datasetMap(fullChunk)
		if len(chunkMap) == 0 {
			lastSampleErr = fmt.Errorf("sampled chunk %s was not found in dataset %s", chunkID, datasetID)
			continue
		}
		vectorField := datasetGuessVecField(chunkMap)
		vector := datasetAsFloatVec(chunkMap[vectorField])
		samples = append(samples, embeddingCheckSample{
			ChunkID:           chunkID,
			KbID:              datasetID,
			DocID:             datasetString(chunkMap["doc_id"]),
			DocName:           datasetString(chunkMap["docnm_kwd"]),
			VectorField:       vectorField,
			Vector:            vector,
			PageNum:           chunkMap["page_num_int"],
			Position:          chunkMap["position_int"],
			Top:               chunkMap["top_int"],
			ContentWithWeight: datasetString(chunkMap["content_with_weight"]),
			QuestionKeywords:  datasetStringSlice(chunkMap["question_kwd"]),
		})
	}

	if len(samples) == 0 {
		return nil, lastSampleErr
	}
	return samples, nil
}

func (d *DatasetService) verifyEmbeddingAvailability(ctx context.Context, embdID, tenantID string) (bool, string) {
	_, err := service.NewModelFactory().ResolveInfo(permission.WithTenantID(ctx, tenantID), entity.ModelTypeEmbedding, embdID)
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}
