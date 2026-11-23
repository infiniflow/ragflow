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

	embeddingModel, err := service.NewModelFactory().NewEmbeddingModel(permission.WithTenantID(ctx, kb.TenantID), embeddingID)
	if err != nil {
		return nil, common.CodeDataError, err
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

	results := make([]service.EmbeddingCheckResult, 0, len(samples))
	effectiveSimilarities := make([]float64, 0, len(samples))
	sawTitleAndContent := false
	for _, sample := range samples {
		if sample.Vector == nil || len(sample.Vector) == 0 {
			continue
		}

		rawChunk, err := d.docEngine.GetChunk(ctx, fmt.Sprintf("ragflow_%s", kb.TenantID), sample.ChunkID, []string{datasetID})
		if err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to get sampled chunk %s: %w", sample.ChunkID, err)
		}
		chunkMap := datasetMap(rawChunk)
		if len(chunkMap) == 0 {
			return nil, common.CodeServerError, fmt.Errorf("sampled chunk %s was not found", sample.ChunkID)
		}

		title := datasetString(chunkMap["title_tks"])
		content := datasetString(chunkMap["content_ltks"])

		var titleVector [][]float64
		if title != "" {
			titleVector, err = datasetEncodeEmbedding(ctx, embeddingModel, []string{title})
			if err != nil {
				return nil, common.CodeServerError, err
			}
		}
		var contentVector [][]float64
		if content != "" {
			contentVector, err = datasetEncodeEmbedding(ctx, embeddingModel, []string{content})
			if err != nil {
				return nil, common.CodeServerError, err
			}
		}

		var vectors [][]float64
		if len(titleVector) > 0 && len(contentVector) > 0 {
			vectors = [][]float64{titleVector[0], contentVector[0]}
			sawTitleAndContent = true
		} else if len(titleVector) > 0 {
			vectors = titleVector
		} else if len(contentVector) > 0 {
			vectors = contentVector
		} else {
			continue
		}

		if len(vectors[0]) != len(sample.Vector) {
			return nil, common.CodeDataError, fmt.Errorf("Embedding failure. The dimension (%d) of given embedding model is different from the original (%d)", len(vectors[0]), len(sample.Vector))
		}

		var sim float64
		if len(vectors) == 2 {
			simContent := datasetCosSim(vectors[1], sample.Vector)
			simMix := datasetCosSim(datasetMixVectors(vectors[0], vectors[1], 0.1), sample.Vector)
			sim = simContent
			if simMix > sim {
				sim = simMix
				sawTitleAndContent = true
			}
		} else {
			sim = datasetCosSim(vectors[0], sample.Vector)
		}
		sim = datasetRoundFloat(sim, 6)

		effectiveSimilarities = append(effectiveSimilarities, sim)
		results = append(results, service.EmbeddingCheckResult{
			ChunkID:     sample.ChunkID,
			DocID:       sample.DocID,
			DocName:     sample.DocName,
			VectorField: sample.VectorField,
			VectorDim:   len(sample.Vector),
			CosSim:      sim,
		})
	}

	// Aggregate the batch mode explicitly: title_and_content when any sample was
	// matched against the title+content mix, content_only otherwise.
	matchMode := "content_only"
	if sawTitleAndContent {
		matchMode = "title_and_content"
	}
	summary := datasetEmbeddingCheckSummary(datasetID, embeddingID, len(samples), effectiveSimilarities, matchMode)
	response := &service.EmbeddingCheckResponse{Summary: summary, Results: results}
	if len(effectiveSimilarities) == 0 {
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
	embeddingModel, err := service.NewModelFactory().NewEmbeddingModel(permission.WithTenantID(ctx, tenantID), embeddingID)
	if err != nil {
		return 0, err
	}
	vectors, err := datasetEncodeEmbedding(ctx, embeddingModel, []string{embeddingSwitchProbeText})
	if err != nil {
		return 0, err
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return 0, fmt.Errorf("embedding model %q returned no vector", embeddingID)
	}
	return len(vectors[0]), nil
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

// storedVectorDimension returns the dimension of the vector one stored chunk of
// the dataset carries, together with the column it lives in. A dimension of 0
// means no chunk of the dataset holds a vector.
//
// A dataset that somehow carries several vector columns answers with whichever
// column the engine enumerates first: the columns can only disagree after a
// switch this check let through, and any of them still tells whether the
// candidate lands in the space the dataset is indexed in.
func (d *DatasetService) storedVectorDimension(ctx context.Context, tenantID, datasetID string) (int, string, error) {
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	found, err := d.docEngine.Search(ctx, &enginetypes.SearchRequest{
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
		return 0, "", fmt.Errorf("failed to sample a chunk: %w", err)
	}
	if found == nil || len(found.Chunks) == 0 {
		return 0, "", nil
	}
	chunkID := datasetChunkID(found.Chunks[0])
	if chunkID == "" {
		return 0, "", nil
	}

	// The search returns the projected fields only, so the vector column is
	// read from the chunk itself.
	raw, err := d.docEngine.GetChunk(ctx, indexName, chunkID, []string{datasetID})
	if err != nil {
		return 0, "", fmt.Errorf("failed to load chunk %s: %w", chunkID, err)
	}
	chunk := datasetMap(raw)
	field := datasetGuessVecField(chunk)
	if field == "" {
		return 0, "", nil
	}
	vector := datasetAsFloatVec(chunk[field])
	if len(vector) == 0 {
		return 0, "", nil
	}
	return len(vector), field, nil
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
