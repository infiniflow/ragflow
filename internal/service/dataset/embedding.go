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
	"ragflow/internal/entity/models"
	"ragflow/internal/service"

	enginetypes "ragflow/internal/engine/types"
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
func (d *DatasetService) CheckEmbedding(ctx context.Context, userID, datasetID string, req *service.CheckEmbeddingRequest) (*service.EmbeddingCheckResponse, common.ErrorCode, error) {
	if datasetID == "" {
		return nil, common.CodeDataError, errors.New(`lack of "Dataset ID"`)
	}
	if !d.kbDAO.Accessible(ctx, dao.DB, datasetID, userID) {
		return nil, common.CodeDataError, errors.New("no authorization")
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

	driver, modelName, apiConfig, maxTokens, err := service.NewModelProviderService().ResolveModelConfig(ctx, kb.TenantID, entity.ModelTypeEmbedding, embeddingID)
	if err != nil {
		return nil, common.CodeDataError, err
	}
	embeddingModel := models.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)

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

func (d *DatasetService) verifyEmbeddingAvailability(ctx context.Context, embdID string, tenantID string) (bool, string) {
	_, _, _, _, err := service.NewModelProviderService().ResolveModelConfig(ctx, tenantID, entity.ModelTypeEmbedding, embdID)
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}
