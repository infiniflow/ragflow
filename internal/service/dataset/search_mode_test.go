package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"
)

type modeSearchEngine struct {
	fakeChatDocEngine
	requests []*types.SearchRequest
}

func (e *modeSearchEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.requests = append(e.requests, req)
	return &types.SearchResult{Chunks: []map[string]interface{}{}}, nil
}

func newModeSearchFixture(t *testing.T, embeddingIDs ...string) (*DatasetService, *modeSearchEngine, []string) {
	t.Helper()
	if err := nlp.InitQueryBuilder(""); err != nil {
		t.Fatalf("初始化检索查询构造器失败：%v", err)
	}
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Search{}); err != nil {
		t.Fatalf("创建搜索配置表失败：%v", err)
	}
	ids := make([]string, len(embeddingIDs))
	for i, embeddingID := range embeddingIDs {
		id := "kb-" + string(rune('a'+i))
		ids[i] = id
		insertDatasetUpdateKB(t, id, "user-1", id)
		if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", id).Update("embd_id", embeddingID).Error; err != nil {
			t.Fatalf("设置知识库模型失败：%v", err)
		}
	}
	eng := &modeSearchEngine{}
	return &DatasetService{kbDAO: dao.NewKnowledgebaseDAO(), documentDAO: dao.NewDocumentDAO(), searchService: service.NewSearchService(), docEngine: eng}, eng, ids
}

func saveModeSearchConfig(t *testing.T, config entity.JSONMap) string {
	t.Helper()
	id := "search-1"
	if err := dao.DB.Create(&entity.Search{
		ID: id, TenantID: "user-1", CreatedBy: "user-1", Name: "检索模式测试", SearchConfig: config, Status: sptr(string(entity.StatusValid)),
	}).Error; err != nil {
		t.Fatalf("保存检索配置失败：%v", err)
	}
	return id
}

func TestSearchRequestRetrievalModeJSON(t *testing.T) {
	for _, kind := range []string{"single", "multiple"} {
		for _, value := range []string{`"hybrid"`, `"vector"`, `"keyword"`, `null`, `""`, `"unknown"`, `"KEYWORD"`, `3`, `true`, `{}`, `[]`} {
			t.Run(kind+"/"+value, func(t *testing.T) {
				var target interface{} = &service.SearchDatasetRequest{}
				if kind == "multiple" {
					target = &service.SearchDatasetsRequest{}
				}
				err := json.Unmarshal([]byte(`{"retrieval_mode":`+value+`}`), target)
				valid := value == `"hybrid"` || value == `"vector"` || value == `"keyword"`
				if (err == nil) != valid {
					t.Fatalf("retrieval_mode=%s，解码错误=%v，合法值=%v", value, err, valid)
				}
			})
		}
	}
	var omitted service.SearchDatasetsRequest
	if err := json.Unmarshal([]byte(`{"question":"测试"}`), &omitted); err != nil || omitted.RetrievalMode != "" {
		t.Fatalf("省略模式时应保留零值：%#v，%v", omitted, err)
	}
}

func TestSearchDatasetsKeywordHasNoEmbeddingDependency(t *testing.T) {
	svc, eng, ids := newModeSearchFixture(t, "missing-one@Unavailable", "missing-two@OtherProvider", "")
	req := &service.SearchDatasetsRequest{
		DatasetIDs: ids, Question: "错误码检索", RetrievalMode: nlp.RetrievalModeKeyword,
		DocumentIDs: []string{"doc-1"}, IncludeCompiledChunks: new(false),
	}
	resp, err := svc.SearchDatasets(t.Context(), req, "user-1")
	if err != nil || resp == nil || resp.Total != 0 {
		t.Fatalf("关键词模式应在异构且不可用的 embedding 配置下完成检索：%#v，%v", resp, err)
	}
	if len(eng.requests) != 1 {
		t.Fatalf("引擎调用次数=%d，期望 1", len(eng.requests))
	}
	got := eng.requests[0]
	if !reflect.DeepEqual(got.KbIDs, ids) || !reflect.DeepEqual(got.Filter["doc_id"], []string{"doc-1"}) || got.Filter["available_int"] != 1 {
		t.Fatalf("检索范围发生变化：%#v", got)
	}
	if !reflect.DeepEqual(got.Filter["must_not"], map[string]interface{}{"exists": "compile_kwd"}) {
		t.Fatalf("编译分块过滤丢失：%#v", got.Filter)
	}
	if len(got.MatchExprs) != 1 {
		t.Fatalf("关键词模式只应生成一个词法表达式：%#v", got.MatchExprs)
	}
	if _, ok := got.MatchExprs[0].(*types.MatchTextExpr); !ok {
		t.Fatalf("关键词表达式类型错误：%T", got.MatchExprs[0])
	}
}

func TestSearchDatasetsRetrievalModePrecedence(t *testing.T) {
	tests := []struct {
		name         string
		requestMode  nlp.RetrievalMode
		saved        entity.JSONMap
		embeddingID  string
		wantArgument bool
		wantModelErr bool
	}{
		{name: "默认混合保留无模型词法检索", embeddingID: ""},
		{name: "默认混合仍检查配置模型", embeddingID: "broken-model", wantModelErr: true},
		{name: "请求关键词覆盖默认", requestMode: nlp.RetrievalModeKeyword, embeddingID: "broken-model"},
		{name: "保存配置缺模式沿用请求", requestMode: nlp.RetrievalModeKeyword, saved: entity.JSONMap{}, embeddingID: "broken-model"},
		{name: "保存配置缺模式沿用默认", saved: entity.JSONMap{}, embeddingID: "broken-model", wantModelErr: true},
		{name: "保存关键词覆盖请求向量", requestMode: nlp.RetrievalModeVector, saved: entity.JSONMap{"retrieval_mode": "keyword"}, embeddingID: "broken-model"},
		{name: "保存向量覆盖请求关键词", requestMode: nlp.RetrievalModeKeyword, saved: entity.JSONMap{"retrieval_mode": "vector"}, wantArgument: true},
		{name: "保存混合覆盖请求关键词", requestMode: nlp.RetrievalModeKeyword, saved: entity.JSONMap{"retrieval_mode": "hybrid"}, embeddingID: "broken-model", wantModelErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, eng, ids := newModeSearchFixture(t, tt.embeddingID)
			req := &service.SearchDatasetsRequest{DatasetIDs: ids, Question: "检索", RetrievalMode: tt.requestMode}
			if tt.saved != nil {
				searchID := saveModeSearchConfig(t, tt.saved)
				req.SearchID = &searchID
			}
			_, err := svc.SearchDatasets(t.Context(), req, "user-1")
			if tt.wantArgument {
				var coded *common.CodedError
				if !errors.As(err, &coded) || coded.Code != common.CodeArgumentError {
					t.Fatalf("期望参数错误，实际=%v", err)
				}
			} else if tt.wantModelErr {
				if err == nil || !strings.Contains(err.Error(), "failed to get embedding model") {
					t.Fatalf("期望模型配置错误，实际=%v", err)
				}
			} else if err != nil {
				t.Fatalf("检索失败：%v", err)
			}
			wantCalls := 1
			if tt.wantArgument || tt.wantModelErr {
				wantCalls = 0
			}
			if len(eng.requests) != wantCalls {
				t.Fatalf("引擎调用=%d，期望=%d", len(eng.requests), wantCalls)
			}
		})
	}
}

func TestSearchDatasetsRejectsInvalidSavedRetrievalMode(t *testing.T) {
	for _, value := range []interface{}{nil, "", "unknown", "KEYWORD", 1, true, []interface{}{}, map[string]interface{}{}} {
		t.Run(string(mustJSON(t, value)), func(t *testing.T) {
			svc, eng, ids := newModeSearchFixture(t, "")
			searchID := saveModeSearchConfig(t, entity.JSONMap{"retrieval_mode": value})
			_, err := svc.SearchDatasets(t.Context(), &service.SearchDatasetsRequest{DatasetIDs: ids, Question: "检索", RetrievalMode: nlp.RetrievalModeKeyword, SearchID: &searchID}, "user-1")
			var coded *common.CodedError
			if !errors.As(err, &coded) || coded.Code != common.CodeArgumentError || !strings.Contains(coded.Message, "retrieval_mode") {
				t.Fatalf("保存模式非法时应返回明确参数错误：%v", err)
			}
			if len(eng.requests) != 0 {
				t.Fatalf("非法模式仍触发 %d 次查询", len(eng.requests))
			}
		})
	}
}

func mustJSON(t *testing.T, value interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("序列化测试值失败：%v", err)
	}
	return data
}

func TestSearchDatasetsModePreservesPermissionAndNoMatchScope(t *testing.T) {
	for _, mode := range []nlp.RetrievalMode{nlp.RetrievalModeHybrid, nlp.RetrievalModeVector, nlp.RetrievalModeKeyword} {
		t.Run(string(mode)+"/权限", func(t *testing.T) {
			svc, eng, ids := newModeSearchFixture(t, "")
			_, err := svc.SearchDatasets(t.Context(), &service.SearchDatasetsRequest{DatasetIDs: ids, Question: "检索", RetrievalMode: mode}, "other-user")
			if !permissionresponse.IsPermissionError(err) || len(eng.requests) != 0 {
				t.Fatalf("越权请求结果=%v，引擎调用=%d", err, len(eng.requests))
			}
		})
	}
	t.Run("关键词保留元数据无匹配标记", func(t *testing.T) {
		svc, eng, ids := newModeSearchFixture(t, "missing-model")
		_, err := svc.SearchDatasets(t.Context(), &service.SearchDatasetsRequest{DatasetIDs: ids, Question: "检索", RetrievalMode: nlp.RetrievalModeKeyword, DocumentIDs: []string{service.NoMatchDocIDSentinel}, IncludeCompiledChunks: new(false)}, "user-1")
		if err != nil || len(eng.requests) != 1 || !reflect.DeepEqual(eng.requests[0].Filter["doc_id"], []string{service.NoMatchDocIDSentinel}) {
			t.Fatalf("无匹配文档范围丢失：请求=%#v，错误=%v", eng.requests, err)
		}
	})
}

func TestSearchDatasetsKeywordKeepsCompilationScope(t *testing.T) {
	for _, tc := range []struct {
		name           string
		scope          string
		include        *bool
		documentID     string
		withDatasetIDs bool
	}{
		{name: "默认范围", documentID: "doc-1", withDatasetIDs: true},
		{name: "知识库制品", scope: "dataset", documentID: "doc-1", withDatasetIDs: true},
		{name: "文档制品", scope: "document", documentID: "doc-1"},
		{name: "全部制品", scope: "all", documentID: "doc-1", withDatasetIDs: true},
		{name: "关闭知识编译", scope: "all", include: new(false), documentID: "doc-1"},
		{name: "默认编译保留无匹配标记", documentID: service.NoMatchDocIDSentinel, withDatasetIDs: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, eng, ids := newModeSearchFixture(t, "missing-model")
			_, err := svc.SearchDatasets(t.Context(), &service.SearchDatasetsRequest{
				DatasetIDs: ids, Question: "检索", RetrievalMode: nlp.RetrievalModeKeyword,
				DocumentIDs: []string{tc.documentID}, IncludeCompiledChunks: tc.include, KnowledgeCompilationScope: tc.scope,
			}, "user-1")
			if err != nil || len(eng.requests) != 1 {
				t.Fatalf("检索错误=%v，请求=%#v", err, eng.requests)
			}
			wantDocIDs := []string{tc.documentID}
			if tc.withDatasetIDs {
				wantDocIDs = append(wantDocIDs, ids...)
			}
			got := eng.requests[0]
			if !reflect.DeepEqual(got.KbIDs, ids) || !reflect.DeepEqual(got.Filter["doc_id"], wantDocIDs) || got.Filter["available_int"] != 1 {
				t.Fatalf("关键词模式改变检索范围：%#v", got)
			}
			wantFilter := knowledgeCompilationFilter(tc.include, tc.scope)
			for _, key := range []string{"or", "must_not"} {
				if !reflect.DeepEqual(got.Filter[key], wantFilter[key]) {
					t.Fatalf("知识编译过滤丢失：字段=%s，实际=%#v，期望=%#v", key, got.Filter[key], wantFilter[key])
				}
			}
		})
	}
}
