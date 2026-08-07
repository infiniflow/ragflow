package models

import (
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func TestProviderEmbeddingAndRerankUsage(t *testing.T) {
	withSSRFBypass(t)
	type providerCase struct {
		name           string
		newModel       func(string) ModelDriver
		embeddingBody  string
		rerankBody     string
		embedInput     int
		embedTotal     int
		rerankInput    int
		rerankOutput   int
		rerankTotal    int
		supportsRerank bool
	}

	cases := []providerCase{
		{
			name: "siliconflow",
			newModel: func(baseURL string) ModelDriver {
				return NewSiliconflowModel(map[string]string{"default": baseURL}, URLSuffix{Embedding: "embedding", Rerank: "rerank"})
			},
			embeddingBody: `{"id":"embed-sf","data":[{"embedding":[0.1,0.2],"index":0}],"usage":{"prompt_tokens":7,"total_tokens":7}}`,
			rerankBody:    `{"id":"rerank-sf","results":[{"index":0,"relevance_score":0.9}],"meta":{"tokens":{"input_tokens":7,"output_tokens":2}}}`,
			embedInput:    7, embedTotal: 7, rerankInput: 7, rerankOutput: 2, rerankTotal: 9,
			supportsRerank: true,
		},
		{
			name: "aliyun",
			newModel: func(baseURL string) ModelDriver {
				return NewAliyunModel(map[string]string{"default": baseURL}, URLSuffix{Embedding: "embedding", Rerank: "rerank"})
			},
			embeddingBody: `{"id":"embed-aliyun","data":[{"embedding":[0.1,0.2],"index":0}],"usage":{"prompt_tokens":7,"total_tokens":7}}`,
			rerankBody:    `{"id":"rerank-aliyun","results":[{"index":0,"relevance_score":0.9}],"usage":{"total_tokens":9}}`,
			embedInput:    7, embedTotal: 7, rerankTotal: 9,
			supportsRerank: true,
		},
		{
			name: "huaweicloud",
			newModel: func(baseURL string) ModelDriver {
				return NewHuaweiCloudModel(map[string]string{"default": baseURL}, URLSuffix{Embedding: "embedding", Rerank: "rerank"})
			},
			embeddingBody: `{"id":"embed-huawei","data":[{"embedding":[0.1,0.2],"index":0}],"usage":{"prompt_tokens":7,"total_tokens":7}}`,
			rerankBody:    `{"id":"rerank-huawei","results":[{"index":0,"relevance_score":0.9}],"usage":{"prompt_tokens":9,"total_tokens":9}}`,
			embedInput:    7, embedTotal: 7, rerankInput: 9, rerankTotal: 9,
			supportsRerank: true,
		},
		{
			name: "volcengine",
			newModel: func(baseURL string) ModelDriver {
				return NewVolcEngine(map[string]string{"default": baseURL}, URLSuffix{Embedding: "embedding"})
			},
			embeddingBody: `{"id":"embed-volc","data":{"embedding":[0.1,0.2]},"usage":{"prompt_tokens":7,"total_tokens":7}}`,
			embedInput:    7, embedTotal: 7,
		},
		{
			name: "jina",
			newModel: func(baseURL string) ModelDriver {
				return NewJinaModel(map[string]string{"default": baseURL}, URLSuffix{Embedding: "embedding", Rerank: "rerank"})
			},
			embeddingBody:  `{"id":"embed-jina","data":[{"embedding":[0.1,0.2],"index":0}],"usage":{"prompt_tokens":7,"total_tokens":7}}`,
			rerankBody:     `{"id":"rerank-jina","results":[{"index":0,"relevance_score":0.9}],"usage":{"prompt_tokens":7,"total_tokens":9}}`,
			embedInput:     7,
			embedTotal:     7,
			rerankInput:    7,
			rerankOutput:   0,
			rerankTotal:    9,
			supportsRerank: true,
		},
		{
			name: "gitee",
			newModel: func(baseURL string) ModelDriver {
				return NewGiteeModel(map[string]string{"default": baseURL}, URLSuffix{Embedding: "embedding", Rerank: "rerank"})
			},
			embeddingBody:  `{"id":"embed-gitee","data":[{"embedding":[0.1,0.2],"index":0}],"usage":{"prompt_tokens":7,"total_tokens":7}}`,
			rerankBody:     `{"id":"rerank-gitee","results":[{"index":0,"relevance_score":0.9}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`,
			embedInput:     7,
			embedTotal:     7,
			rerankInput:    7,
			rerankOutput:   2,
			rerankTotal:    9,
			supportsRerank: true,
		},
		{
			name: "openrouter",
			newModel: func(baseURL string) ModelDriver {
				return NewOpenRouterModel(map[string]string{"default": baseURL}, URLSuffix{Embedding: "embedding", Rerank: "rerank"})
			},
			embeddingBody:  `{"id":"embed-openrouter","data":[{"embedding":[0.1,0.2],"index":0}],"usage":{"prompt_tokens":7,"total_tokens":7}}`,
			rerankBody:     `{"id":"rerank-openrouter","results":[{"index":0,"relevance_score":0.9}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`,
			embedInput:     7,
			embedTotal:     7,
			rerankInput:    7,
			rerankOutput:   2,
			rerankTotal:    9,
			supportsRerank: true,
		},
		{
			name: "jiekouai",
			newModel: func(baseURL string) ModelDriver {
				return NewJieKouAIModel(map[string]string{"default": baseURL}, URLSuffix{Embedding: "embedding", Rerank: "rerank"})
			},
			embeddingBody:  `{"id":"embed-jiekouai","data":[{"embedding":[0.1,0.2],"index":0}],"usage":{"prompt_tokens":7,"total_tokens":7}}`,
			rerankBody:     `{"id":"rerank-jiekouai","results":[{"index":0,"relevance_score":0.9}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`,
			embedInput:     7,
			embedTotal:     7,
			rerankInput:    7,
			rerankOutput:   2,
			rerankTotal:    9,
			supportsRerank: true,
		},
		{
			name: "hunyuan",
			newModel: func(baseURL string) ModelDriver {
				return NewHunyuanModel(map[string]string{"default": baseURL}, URLSuffix{Embedding: "embedding"})
			},
			embeddingBody: `{"id":"embed-hunyuan","data":[{"embedding":[0.1,0.2],"index":0}],"usage":{"prompt_tokens":7,"total_tokens":7}}`,
			embedInput:    7,
			embedTotal:    7,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/embedding":
					_, _ = w.Write([]byte(tc.embeddingBody))
				case "/rerank":
					_, _ = w.Write([]byte(tc.rerankBody))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			apiKey := "test-key"
			modelName := "test-model"
			model := tc.newModel(server.URL)

			embeddingUsage := &common.ModelUsage{}
			embeddings, err := model.Embed(t.Context(), &modelName, []string{"document"}, &APIConfig{ApiKey: &apiKey}, &EmbeddingConfig{}, embeddingUsage)
			if err != nil {
				t.Fatalf("Embed: %v", err)
			}
			if len(embeddings) != 1 || len(embeddings[0].Embedding) != 2 {
				t.Fatalf("embeddings=%#v", embeddings)
			}
			assertModelUsage(t, embeddingUsage, tc.embedInput, 0, tc.embedTotal)
			if embeddingUsage.Type != "embedding" {
				t.Fatalf("embedding usage type=%q, want embedding", embeddingUsage.Type)
			}
			if !tc.supportsRerank {
				return
			}

			rerankUsage := &common.ModelUsage{}
			reranked, err := model.Rerank(t.Context(), &modelName, "query", []string{"document"}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{TopN: 1}, rerankUsage)
			if err != nil {
				t.Fatalf("Rerank: %v", err)
			}
			if len(reranked.Data) != 1 || reranked.Data[0].Index != 0 {
				t.Fatalf("reranked=%#v", reranked)
			}
			assertModelUsage(t, rerankUsage, tc.rerankInput, tc.rerankOutput, tc.rerankTotal)
			if rerankUsage.Type != "rerank" {
				t.Fatalf("rerank usage type=%q, want rerank", rerankUsage.Type)
			}
		})
	}
}

func assertModelUsage(t *testing.T, usage *common.ModelUsage, input, output, total int) {
	t.Helper()
	if usage.RequestID == "" {
		t.Fatal("usage request ID is empty")
	}
	if usage.InputTokens != input || usage.OutputTokens != output || usage.TotalTokens != total {
		t.Fatalf("usage=(%d,%d,%d), want (%d,%d,%d)", usage.InputTokens, usage.OutputTokens, usage.TotalTokens, input, output, total)
	}
}
