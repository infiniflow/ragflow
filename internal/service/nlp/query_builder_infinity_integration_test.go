//go:build integration

package nlp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"ragflow/internal/tokenizer"
)

// TestQueryBuilderInfinityChineseFullText exercises query construction against
// Infinity's rag-coarse/rag-fine indexes. Set INFINITY_HTTP_URL to a test server.
func TestQueryBuilderInfinityChineseFullText(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("INFINITY_HTTP_URL"), "/")
	if baseURL == "" {
		t.Skip("set INFINITY_HTTP_URL to run against Infinity")
	}
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })
	client := &http.Client{Timeout: 30 * time.Second}
	request := func(t *testing.T, method, path string, body any) [][]map[string]any {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(method, baseURL+path, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var result struct {
			ErrorCode int                `json:"error_code"`
			ErrorMsg  string             `json:"error_msg"`
			Output    [][]map[string]any `json:"output"`
		}
		if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusOK || result.ErrorCode != 0 {
			t.Fatalf("%s %s: HTTP %d, Infinity %d: %s", method, path, res.StatusCode, result.ErrorCode, result.ErrorMsg)
		}
		return result.Output
	}

	tablePath := fmt.Sprintf("/databases/default_db/tables/query_regression_%d", time.Now().UnixNano())
	request(t, http.MethodPost, tablePath, map[string]any{
		"create_option": "error",
		"fields": []map[string]string{
			{"name": "id", "type": "varchar"},
			{"name": "docnm", "type": "varchar"},
			{"name": "important_keywords", "type": "varchar"},
			{"name": "questions", "type": "varchar"},
			{"name": "content", "type": "varchar"},
		},
	})
	t.Cleanup(func() {
		request(t, http.MethodDelete, tablePath, map[string]string{"drop_option": "error"})
	})
	for _, column := range []string{"docnm", "important_keywords", "questions", "content"} {
		for _, grain := range []string{"coarse", "fine"} {
			index := "ft_" + column + "_rag_" + grain
			request(t, http.MethodPost, tablePath+"/indexes/"+index, map[string]any{
				"create_option": "error",
				"fields":        []string{column},
				"index": map[string]any{
					"type":     "fulltext",
					"analyzer": "rag-" + grain,
				},
			})
		}
	}
	request(t, http.MethodPost, tablePath+"/docs", []map[string]string{
		{"id": "answer", "docnm": "青柚七号巡检机器人 技术说明", "important_keywords": "", "questions": "", "content": "机器人搭载一台 640×512 分辨率的红外热像仪，用于发现电缆接头过热；另配一套激光甲烷检测仪，报警阈值设定为 1.5% LEL。"},
		{"id": "unrelated", "docnm": "", "important_keywords": "", "questions": "", "content": "今天食堂供应番茄炒蛋和米饭。"},
	})
	qb := NewQueryBuilder()
	for _, question := range []string{"甲烷报警阈值是多少", "红外热像仪", "LEL"} {
		t.Run(question, func(t *testing.T) {
			expr, _ := qb.Question(question, "", 0.3)
			if expr == nil {
				t.Fatal("missing full-text expression")
			}
			rows := request(t, http.MethodGet, tablePath+"/docs", map[string]any{
				"output": []string{"id", "score()"},
				"search": []map[string]any{{
					"match_method":  "text",
					"fields":        "docnm@ft_docnm_rag_coarse^10,docnm@ft_docnm_rag_fine^5,important_keywords@ft_important_keywords_rag_coarse^30,important_keywords@ft_important_keywords_rag_fine^20,questions@ft_questions_rag_fine^20,content@ft_content_rag_coarse^2,content@ft_content_rag_fine",
					"matching_text": expr.MatchingText,
					"topn":          10,
					"params":        map[string]string{"minimum_should_match": "30%"},
				}},
			})
			if len(rows) != 1 || len(rows[0]) != 2 || rows[0][0]["id"] != "answer" {
				t.Fatalf("query %s returned %v, want only answer", expr.MatchingText, rows)
			}
			if score, ok := rows[0][1]["SCORE"].(float64); !ok || score <= 0 {
				t.Fatalf("query %s has no positive full-text score: %v", expr.MatchingText, rows)
			}
			t.Logf("query %s returned %v", expr.MatchingText, rows)
		})
	}
}
