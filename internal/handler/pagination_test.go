package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	dataset "ragflow/internal/service/dataset"
)

func TestListHandlersRejectInvalidPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range map[string]gin.HandlerFunc{
		"datasets":           (&DatasetsHandler{}).ListDatasets,
		"ingestion logs":     (&DatasetsHandler{}).ListIngestionLogs,
		"chats":              (&ChatHandler{}).ListChats,
		"chat sessions":      (&ChatSessionHandler{}).ListChatSessions,
		"agents":             (&AgentHandler{}).ListAgents,
		"agent sessions":     (&AgentHandler{}).ListAgentSessions,
		"searches":           (&SearchHandler{}).ListSearches,
		"models":             (&ModelHandler{}).ListAllModels,
		"files":              (&FileHandler{}).ListFiles,
		"documents":          (&DocumentHandler{}).ListDocuments,
		"chunks":             (&ChunkHandler{}).ListChunks,
		"memories":           (&MemoryHandler{}).ListMemories,
		"memory messages":    (&MemoryHandler{}).GetMemoryMessages,
		"connector logs":     (&ConnectorHandler{}).ListLogs,
		"sync logs":          (&ConnectorHandler{}).ListSyncLogs,
		"MCP servers":        (&MCPHandler{}).ListMCPServers,
		"compilation groups": (&CompilationTemplateGroupHandler{}).List,
	} {
		for _, query := range []string{"page_size=101", "page_size=0", "page_size=-1", "page_size=invalid", "page_size=1&page_size=101"} {
			t.Run(name+"/"+query, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
				c.Set("user", &entity.User{ID: "user-1"})
				c.Params = gin.Params{{Key: "chat_id", Value: "chat-1"}, {Key: "dataset_id", Value: "ds-1"}, {Key: "document_id", Value: "doc-1"}, {Key: "memory_id", Value: "mem-1"}}
				handle(c)
				var response struct {
					Code    common.ErrorCode `json:"code"`
					Message string           `json:"message"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Code == common.CodeSuccess || !strings.Contains(response.Message, "page_size") {
					t.Fatalf("unexpected response: %s", rec.Body.String())
				}
			})
		}
	}
}

func TestSearchHandlersRejectOversizedJSONPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, handle := range map[string]gin.HandlerFunc{
		"dataset":  (&DatasetsHandler{}).SearchDataset,
		"datasets": (&DatasetsHandler{}).SearchDatasets,
	} {
		for _, fields := range []string{`"page_size":101`, `"size":101`, `"page_size":10,"size":101`, `"page_size":0`} {
			t.Run(name+"/"+fields, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"question":"hello","dataset_ids":["ds-1"],`+fields+`}`))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set("user", &entity.User{ID: "user-1"})
				c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}
				handle(c)
				var response struct {
					Code    common.ErrorCode `json:"code"`
					Message string           `json:"message"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Code == common.CodeSuccess || !strings.Contains(response.Message, "size must be between 1 and 100") {
					t.Fatalf("unexpected response: %s", rec.Body.String())
				}
			})
		}
	}
}

func TestRetrievalAndSkillSearchRejectOversizedPagination(t *testing.T) {
	for _, tt := range []struct {
		name   string
		handle gin.HandlerFunc
		body   string
	}{
		{"retrieval", (&SearchBotHandler{}).RetrievalTest, `{"question":"hello","kb_ids":["ds-1"],"size":101}`},
		{"skills", (&SkillSearchHandler{}).Search, `{"query":"hello","page_size":101}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", &entity.User{ID: "user-1"})
			tt.handle(c)
			var response struct {
				Code    common.ErrorCode `json:"code"`
				Message string           `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code == common.CodeSuccess || !strings.Contains(response.Message, "size must be between 1 and 100") {
				t.Fatalf("unexpected response: %s", rec.Body.String())
			}
		})
	}
}

func TestCommitsRejectOversizedPagination(t *testing.T) {
	originalDB := dao.DB
	t.Cleanup(func() { dao.DB = originalDB })
	router, _ := setupFileCommitTest("user-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/folders/folder-1/commits?page_size=101", nil))
	var response struct {
		Code    common.ErrorCode `json:"code"`
		Message string           `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code == common.CodeSuccess || response.Message != "page_size must be between 1 and 100" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestArtifactsRejectOversizedPagination(t *testing.T) {
	const datasetID = "123e4567e89b12d3a456426614174000"
	db := setupArtifactGraphDB(t)
	insertArtifactGraphKB(t, db, datasetID, "user-1")
	h := &DatasetArtifactHandler{datasetSvc: dataset.NewDatasetService()}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/?page_size=101", nil)
	c.Request = req.WithContext(permission.WithIdentity(req.Context(), "user-1", "user-1"))
	c.Set("user", &entity.User{ID: "user-1"})
	c.Params = gin.Params{{Key: "dataset_id", Value: datasetID}}
	h.ListArtifacts(c)
	var response struct {
		Code    common.ErrorCode `json:"code"`
		Message string           `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code == common.CodeSuccess || response.Message != "page_size must be between 1 and 100" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}
