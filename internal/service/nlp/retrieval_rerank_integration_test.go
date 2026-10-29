//go:build integration

package nlp

import (
	"context"
	"math"
	"os"
	"slices"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity/models"
)

// Uses the Elasticsearch/TEI fixture in retrieval_integration_test.go plus
// SILICONFLOW_API_KEY for the fixed BAAI/bge-reranker-v2-m3 model.
// SILICONFLOW_BASE_URL may select another deployment of the same provider.
// Missing required configuration skips these tests; configured service failures,
// including authentication failures, remain fatal.
// No tests run in parallel: the fixture temporarily installs dao.DB.
const retrievalIntegrationReranker = "BAAI/bge-reranker-v2-m3"

type rerankIntegrationCall struct {
	query     string
	documents []string
	results   []models.RerankResult
}

// Records actual driver traffic without changing the request, response or error.
type rerankIntegrationRecorder struct {
	models.ModelDriver
	calls []rerankIntegrationCall
}

func (r *rerankIntegrationRecorder) Rerank(ctx context.Context, name *string, request models.RerankRequest, api *models.APIConfig, config *models.RerankConfig, usage *common.ModelUsage) (*models.RerankResponse, error) {
	response, err := r.ModelDriver.Rerank(ctx, name, request, api, config, usage)
	call := rerankIntegrationCall{query: request.Query, documents: slices.Clone(request.Documents)}
	if response != nil {
		call.results = slices.Clone(response.Data)
	}
	r.calls = append(r.calls, call)
	return response, err
}

// Catches truncation to page_size before the real model call, missing natural
// text, and returning engine order instead of the real model's highest score.
func TestRetrievalRerankIntegration(t *testing.T) {
	f, model, recorder := newRerankIntegrationFixture(t)
	req := f.request(1, 0)
	req.RerankModel = model
	req.RerankCandidatesCount = new(3)
	req.PageSize = 1
	result := f.retrieve(t, req)
	call := mustHaveRerankIntegrationCall(t, recorder, 1)
	if len(call.documents) != 3 {
		t.Fatalf("reranker received %d candidates, want 3 before page_size=1 truncation", len(call.documents))
	}
	if len(result.Chunks) != 1 || result.Total != 3 {
		t.Fatalf("chunks=%d total=%d, want one returned chunk and three scored candidates", len(result.Chunks), result.Total)
	}
	scores := rerankIntegrationScores(t, call)
	best := math.Inf(-1)
	for _, score := range scores {
		best = math.Max(best, score)
	}
	text, ok := result.Chunks[0]["content_with_weight"].(string)
	score, exists := scores[text]
	if !ok || !exists || math.Abs(score-best) > 1e-6 {
		t.Fatalf("returned chunk is not a highest-scoring real rerank candidate: %v", result.Chunks[0])
	}
	if got := result.Chunks[0]["similarity"].(float64); math.Abs(got-score) > 1e-6 {
		t.Fatalf("final similarity=%g, want real reranker score %g", got, score)
	}
}

// Catches filtering on engine scores instead of rerank scores. Both calls must
// reach the model with the same two candidates before the fixed 0.5 cutoff.
func TestRetrievalRerankThresholdIntegration(t *testing.T) {
	f, model, recorder := newRerankIntegrationFixture(t)
	request := func(threshold float64) *RetrievalRequest {
		req := f.request(1, threshold)
		req.RerankModel = model
		req.DocIDs = []string{"doc-b", "doc-low"}
		return req
	}
	low := f.retrieve(t, request(0))
	mustHaveRetrievalChunks(t, low, []string{"b1", "lexical"})
	first := mustHaveRerankIntegrationCall(t, recorder, 1)
	firstScores := rerankIntegrationScores(t, first)
	high := f.retrieve(t, request(0.5))
	second := mustHaveRerankIntegrationCall(t, recorder, 2)
	secondScores := rerankIntegrationScores(t, second)
	wantTexts := []string{
		retrievalIntegrationQuestion + " spacecraft",
		"Rocket Engines is a restaurant serving peppery salad leaves, fresh tomatoes, garden vegetables and homemade bread.",
	}
	wantSet := slices.Clone(wantTexts)
	slices.Sort(wantSet)
	for _, call := range []rerankIntegrationCall{first, second} {
		got := slices.Clone(call.documents)
		slices.Sort(got)
		if !slices.Equal(got, wantSet) {
			t.Fatalf("model input changed before rerank threshold filtering: got %q, want %q", got, wantSet)
		}
	}
	for _, scores := range []map[string]float64{firstScores, secondScores} {
		if scores[wantTexts[0]] < 0.5 || scores[wantTexts[1]] >= 0.5 {
			t.Fatalf("fixed model/fixture must distinguish relevant and irrelevant passages at 0.5: %v", scores)
		}
	}
	mustHaveRetrievalChunks(t, high, []string{"b1"})
	for _, result := range []*RetrievalResult{low, high} {
		scores := firstScores
		if result == high {
			scores = secondScores
		}
		for _, chunk := range result.Chunks {
			text, _ := chunk["content_with_weight"].(string)
			if score, ok := scores[text]; !ok || math.Abs(chunk["similarity"].(float64)-score) > 1e-6 {
				t.Fatalf("chunk does not carry its real rerank score: %v", chunk)
			}
		}
	}
}

func newRerankIntegrationFixture(t *testing.T) (*retrievalIntegrationFixture, *models.RerankModel, *rerankIntegrationRecorder) {
	t.Helper()
	key := os.Getenv("SILICONFLOW_API_KEY")
	if key == "" {
		t.Skip("set SILICONFLOW_API_KEY to run real reranker integration tests")
	}
	baseURL := os.Getenv("SILICONFLOW_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.siliconflow.cn/v1"
	}
	f := newRetrievalIntegrationFixture(t, "elasticsearch")
	name := retrievalIntegrationReranker
	driver := models.NewSiliconflowModel(map[string]string{"default": baseURL}, models.URLSuffix{Rerank: "rerank"})
	recorder := &rerankIntegrationRecorder{ModelDriver: driver}
	model := models.NewRerankModel(recorder, &name, &models.APIConfig{ApiKey: &key, BaseURL: &baseURL}, 8192)
	return f, model, recorder
}

func mustHaveRerankIntegrationCall(t *testing.T, recorder *rerankIntegrationRecorder, count int) rerankIntegrationCall {
	t.Helper()
	if len(recorder.calls) != count {
		t.Fatalf("real rerank calls=%d, want %d", len(recorder.calls), count)
	}
	call := recorder.calls[count-1]
	if call.query != retrievalIntegrationQuestion {
		t.Fatalf("rerank query=%q, want %q", call.query, retrievalIntegrationQuestion)
	}
	return call
}

func rerankIntegrationScores(t *testing.T, call rerankIntegrationCall) map[string]float64 {
	t.Helper()
	if len(call.results) != len(call.documents) || len(call.documents) == 0 {
		t.Fatalf("real model returned %d scores for %d candidates", len(call.results), len(call.documents))
	}
	seen := make(map[int]bool, len(call.documents))
	scores := make(map[string]float64, len(call.documents))
	for _, result := range call.results {
		if result.Index < 0 || result.Index >= len(call.documents) || seen[result.Index] || math.IsNaN(result.RelevanceScore) || math.IsInf(result.RelevanceScore, 0) || result.RelevanceScore < 0 || result.RelevanceScore > 1 {
			t.Fatalf("invalid calibrated score/index from fixed reranker: %+v", result)
		}
		seen[result.Index] = true
		text := call.documents[result.Index]
		// Match against the natural indexed content, never tokenized text.
		switch text {
		case retrievalIntegrationQuestion,
			retrievalIntegrationQuestion + " vacuum",
			retrievalIntegrationQuestion + " spacecraft",
			"Rocket Engines is a restaurant serving peppery salad leaves, fresh tomatoes, garden vegetables and homemade bread.",
			"A rocket engine produces thrust by ejecting hot gas into space.":
		default:
			t.Fatalf("reranker received altered or tokenized content: %q", text)
		}
		scores[text] = result.RelevanceScore
	}
	return scores
}
