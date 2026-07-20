package dataset

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"
)

func datasetListItemToMap(kb *entity.KnowledgebaseListItem) map[string]interface{} {
	item := map[string]interface{}{
		"id":              kb.ID,
		"name":            kb.Name,
		"tenant_id":       kb.TenantID,
		"permission":      kb.Permission,
		"document_count":  kb.DocNum,
		"token_num":       kb.TokenNum,
		"chunk_count":     kb.ChunkNum,
		"parser_id":       kb.ParserID,
		"embedding_model": kb.EmbdID,
		"nickname":        kb.Nickname,
	}
	if kb.Avatar != nil {
		item["avatar"] = *kb.Avatar
	}
	if kb.Language != nil {
		item["language"] = *kb.Language
	}
	if kb.Description != nil {
		item["description"] = *kb.Description
	}
	if kb.TenantAvatar != nil {
		item["tenant_avatar"] = *kb.TenantAvatar
	}
	if kb.UpdateTime != nil {
		item["update_time"] = *kb.UpdateTime
	}
	return item
}

func datasetToMap(kb *entity.Knowledgebase) map[string]interface{} {
	item := map[string]interface{}{
		"id":                       kb.ID,
		"tenant_id":                kb.TenantID,
		"name":                     kb.Name,
		"embedding_model":          kb.EmbdID,
		"permission":               kb.Permission,
		"created_by":               kb.CreatedBy,
		"document_count":           kb.DocNum,
		"token_num":                kb.TokenNum,
		"chunk_count":              kb.ChunkNum,
		"similarity_threshold":     kb.SimilarityThreshold,
		"vector_similarity_weight": kb.VectorSimilarityWeight,
		"parser_id":                kb.ParserID,
		"parser_config":            kb.ParserConfig,
		"pagerank":                 kb.Pagerank,
		"create_time":              kb.CreateTime,
	}
	if kb.Avatar != nil {
		item["avatar"] = *kb.Avatar
	}
	if kb.Language != nil {
		item["language"] = *kb.Language
	}
	if kb.Description != nil {
		item["description"] = *kb.Description
	}
	if kb.PipelineID != nil {
		item["pipeline_id"] = *kb.PipelineID
	}
	if kb.GraphragTaskID != nil {
		item["graphrag_task_id"] = *kb.GraphragTaskID
	}
	if kb.GraphragTaskFinishAt != nil {
		item["graphrag_task_finish_at"] = kb.GraphragTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.RaptorTaskID != nil {
		item["raptor_task_id"] = *kb.RaptorTaskID
	}
	if kb.RaptorTaskFinishAt != nil {
		item["raptor_task_finish_at"] = kb.RaptorTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.MindmapTaskID != nil {
		item["mindmap_task_id"] = *kb.MindmapTaskID
	}
	if kb.MindmapTaskFinishAt != nil {
		item["mindmap_task_finish_at"] = kb.MindmapTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.UpdateTime != nil {
		item["update_time"] = *kb.UpdateTime
	}
	return item
}

func limitStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}

func stringPointerValue(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

func int64PointerValue(i *int64) interface{} {
	if i == nil {
		return nil
	}
	return *i
}

func timePointerValue(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02 15:04:05")
}

func jsonMapValue(m entity.JSONMap) interface{} {
	if m == nil {
		return nil
	}
	return map[string]interface{}(m)
}

func datasetMap(value interface{}) map[string]interface{} {
	if m, ok := value.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func datasetString(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func datasetStringSlice(value interface{}) []string {
	if sl, ok := value.([]string); ok {
		return sl
	}
	if raw, ok := value.([]interface{}); ok {
		result := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func datasetGuessVecField(src map[string]interface{}) string {
	var f64, f32 string
	for k, v := range src {
		if !strings.HasPrefix(k, "q_") && !strings.HasPrefix(k, "u_") {
			continue
		}
		switch v.(type) {
		case []float64:
			f64 = k
		case string:
			f32 = k
		}
	}
	if f64 != "" {
		return f64
	}
	return f32
}

func datasetAsFloatVec(v interface{}) []float64 {
	switch val := v.(type) {
	case []float64:
		return val
	case []interface{}:
		vec := make([]float64, 0, len(val))
		for _, item := range val {
			switch n := item.(type) {
			case float64:
				vec = append(vec, n)
			case int:
				vec = append(vec, float64(n))
			case int64:
				vec = append(vec, float64(n))
			case json.Number:
				if f, err := n.Float64(); err == nil {
					vec = append(vec, f)
				}
			}
		}
		return vec
	}
	return nil
}

func datasetCosSim(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func datasetCleanEmbeddingText(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(s, "")
}

func datasetEncodeEmbedding(embeddingModel *modelModule.EmbeddingModel, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	cleaned := make([]string, len(texts))
	for i, t := range texts {
		cleaned[i] = datasetCleanEmbeddingText(t)
	}
	embeddingConfig := &modelModule.EmbeddingConfig{Dimension: 0}
	embeddings, err := embeddingModel.ModelDriver.Embed(embeddingModel.ModelName, cleaned, embeddingModel.APIConfig, embeddingConfig, nil)
	if err != nil {
		return nil, err
	}
	vectors := make([][]float64, len(embeddings))
	for i, embedding := range embeddings {
		vectors[i] = embedding.Embedding
	}
	return vectors, nil
}

func datasetMixVectors(titleVector, contentVector []float64, titleWeight float64) []float64 {
	if len(titleVector) == 0 && len(contentVector) == 0 {
		return nil
	}
	if len(titleVector) == 0 {
		return contentVector
	}
	if len(contentVector) == 0 {
		return titleVector
	}
	minLen := len(titleVector)
	if len(contentVector) < minLen {
		minLen = len(contentVector)
	}
	mixed := make([]float64, minLen)
	for i := 0; i < minLen; i++ {
		mixed[i] = titleWeight*titleVector[i] + (1-titleWeight)*contentVector[i]
	}
	return mixed
}

func datasetEmbeddingCheckSummary(datasetID, embeddingID string, sampled int, similarities []float64, matchMode string) service.EmbeddingCheckSummary {
	if len(similarities) == 0 {
		return service.EmbeddingCheckSummary{
			KbID:      datasetID,
			Model:     embeddingID,
			Sampled:   sampled,
			Valid:     0,
			AvgCosSim: 0,
			MinCosSim: 0,
			MaxCosSim: 0,
			MatchMode: matchMode,
		}
	}
	sort.Float64s(similarities)
	var sum float64
	for _, v := range similarities {
		sum += v
	}
	return service.EmbeddingCheckSummary{
		KbID:      datasetID,
		Model:     embeddingID,
		Sampled:   sampled,
		Valid:     len(similarities),
		AvgCosSim: datasetRoundFloat(sum/float64(len(similarities)), 4),
		MinCosSim: datasetRoundFloat(similarities[0], 4),
		MaxCosSim: datasetRoundFloat(similarities[len(similarities)-1], 4),
		MatchMode: matchMode,
	}
}

func datasetRoundFloat(value float64, places int) float64 {
	shift := math.Pow(10, float64(places))
	return math.Round(value*shift) / shift
}

func datasetChunkID(chunk map[string]interface{}) string {
	if id, ok := chunk["chunk_id"]; ok {
		if s, ok := id.(string); ok {
			return s
		}
	}
	return ""
}

func interfaceSlice(items ...string) []interface{} {
	result := make([]interface{}, len(items))
	for i, v := range items {
		result[i] = v
	}
	return result
}
