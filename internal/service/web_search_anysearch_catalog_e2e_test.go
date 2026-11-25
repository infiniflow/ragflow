//go:build e2e

package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

type anySearchSavedCatalogCase struct {
	Name    string `json:"name"`
	Request struct {
		Query      string         `json:"query"`
		Tag        string         `json:"tag,omitempty"`
		Params     map[string]any `json:"params,omitempty"`
		Zone       string         `json:"zone,omitempty"`
		Language   string         `json:"language,omitempty"`
		MaxResults int            `json:"max_results"`
	} `json:"request"`
}

// TestAnySearchSavedCatalogE2E verifies saved Canvas routing and delivery using
// explicit inputs from the current catalog. Source semantics require a separate
// review of each result; transport success is not a source-scope assertion.
func TestAnySearchSavedCatalogE2E(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH_CATALOG_E2E") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH_CATALOG_E2E=1 for saved catalog workflows")
	}
	required := func(name string) string {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			t.Fatalf("%s is required", name)
		}
		return value
	}
	origin := required("RAGFLOW_TEST_API_URL")
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.User != nil || u.RawQuery != "" || (u.Path != "" && u.Path != "/") {
		t.Fatal("RAGFLOW_TEST_API_URL must be a local HTTP API origin")
	}
	token := required("RAGFLOW_TEST_API_TOKEN")
	key := strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY"))
	raw, err := os.ReadFile(required("RAGFLOW_ANYSEARCH_CATALOG_REQUESTS"))
	if err != nil || len(raw) > 4<<20 {
		t.Fatal("cannot read the bounded catalog request matrix")
	}
	for _, secret := range []string{token, key} {
		if secret != "" && bytes.Contains(raw, []byte(secret)) {
			t.Fatal("catalog request matrix must not contain credentials")
		}
	}
	var cases []anySearchSavedCatalogCase
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cases) != nil || decoder.Decode(new(any)) != io.EOF || len(cases) == 0 || len(cases) > 1000 {
		t.Fatal("catalog request matrix must be a nonempty JSON array of at most 1000 cases")
	}
	names := map[string]bool{}
	namePattern := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,159}$`)
	for _, tc := range cases {
		if !namePattern.MatchString(tc.Name) || names[tc.Name] || strings.TrimSpace(tc.Request.Query) == "" || tc.Request.MaxResults < 1 || tc.Request.MaxResults > 10 || (len(tc.Request.Params) > 0 && strings.TrimSpace(tc.Request.Tag) == "") || (tc.Request.Zone != "" && tc.Request.Zone != "cn" && tc.Request.Zone != "intl") {
			t.Fatal("catalog matrix contains an invalid or duplicate case; no workflow created")
		}
		names[tc.Name] = true
	}
	base := &anySearchE2EHTTP{t: t, origin: strings.TrimRight(origin, "/"), token: token, client: &http.Client{Timeout: 2 * time.Minute}}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			h := *base
			h.t = t
			dsl, nodeID := anySearchRealModelDSL("standalone_canvas", "", key, 0)
			components := dsl["components"].(map[string]any)
			params := components[nodeID].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
			params["top_n"] = tc.Request.MaxResults
			inputs := map[string]any{"max_results": map[string]any{"type": "integer", "value": tc.Request.MaxResults}}
			if tc.Request.Tag != "" {
				inputs["tag"] = map[string]any{"type": "line", "value": tc.Request.Tag}
			}
			if tc.Request.Params != nil {
				inputs["params"] = map[string]any{"type": "object", "value": tc.Request.Params}
			}
			if tc.Request.Zone != "" {
				params["zone"] = tc.Request.Zone
			}
			if tc.Request.Language != "" {
				params["language"] = tc.Request.Language
			}
			// Structured output also delivers records that cannot become HTTP
			// references, such as a torrent's magnet link.
			components["Message:live"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)["content"] = []string{"{" + nodeID + "@json}"}
			var created struct {
				ID string `json:"id"`
			}
			h.decode(h.call(http.MethodPost, "/api/v1/agents", map[string]any{"title": "AnySearch catalog " + tc.Name, "canvas_category": "agent_canvas", "dsl": dsl}), &created)
			if created.ID == "" {
				t.Fatal("saved catalog workflow has no ID")
			}
			t.Cleanup(func() { h.cleanup(http.MethodDelete, "/api/v1/agents/"+created.ID, nil) })
			t.Cleanup(func() {
				h.cleanup(http.MethodDelete, "/api/v1/agents/"+created.ID+"/sessions", map[string]any{"delete_all": true})
			})
			var saved struct {
				DSL struct {
					Components map[string]struct {
						Obj struct {
							Params map[string]any `json:"params"`
						} `json:"obj"`
					} `json:"components"`
				} `json:"dsl"`
			}
			h.decode(h.call(http.MethodGet, "/api/v1/agents/"+created.ID, nil), &saved)
			got, want := saved.DSL.Components[nodeID].Obj.Params, params
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)
			if !bytes.Equal(gotJSON, wantJSON) {
				t.Fatal("saved workflow did not preserve the explicit catalog settings")
			}
			route := maps.Clone(got)
			delete(route, "api_key")
			started := time.Now().UTC()
			events := h.stream("/api/v1/agents/chat/completions", map[string]any{"agent_id": created.ID, "query": tc.Request.Query, "inputs": inputs, "stream": true})
			ended := time.Now().UTC()
			var finished map[string]any
			var answer, session string
			for _, event := range events {
				if id, ok := event["session_id"].(string); ok {
					session = id
				}
				data, _ := event["data"].(map[string]any)
				if event["event"] == "message" {
					text, _ := data["content"].(string)
					answer += text
				}
				if event["event"] == "node_finished" && data["component_id"] == nodeID {
					finished = data
				}
			}
			h.receipt("catalog-"+tc.Name+"-run", map[string]any{"started_utc": started.Format(time.RFC3339Nano), "case": tc, "agent_id": created.ID, "saved_params": route, "api_inputs": inputs, "events": events, "scope_verified": false})
			if finished == nil || session == "" || strings.TrimSpace(answer) == "" {
				t.Fatal("saved catalog workflow did not deliver a nonempty saved answer")
			}
			out, _ := finished["outputs"].(map[string]any)
			var queries []struct {
				Index        int                  `json:"index"`
				Query        string               `json:"query"`
				Tag          string               `json:"tag"`
				RequestID    string               `json:"request_id"`
				Error        string               `json:"_ERROR"`
				Results      []map[string]any     `json:"results"`
				ReferenceIDs map[string][2]string `json:"reference_ids"`
			}
			encoded, _ := json.Marshal(out["json"])
			if json.Unmarshal(encoded, &queries) != nil || len(queries) != 1 {
				t.Fatal("saved catalog node did not return exactly one query record")
			}
			q := queries[0]
			if q.Error != "" || q.Index != 0 || q.Query != tc.Request.Query || q.Tag != tc.Request.Tag || q.RequestID == "" || len(q.Results) == 0 || len(q.Results) > tc.Request.MaxResults {
				t.Fatal("saved catalog query failed or lost its mapping/results/request ID")
			}
			var delivered any
			if json.Unmarshal([]byte(answer), &delivered) != nil {
				t.Fatal("user message did not receive the structured catalog result")
			}
			answerJSON, _ := json.Marshal(delivered)
			outputJSON, _ := json.Marshal(out["json"])
			if !bytes.Equal(answerJSON, outputJSON) {
				t.Fatal("user message changed the delivered query, diagnostics or result fields")
			}
			if path := os.Getenv("RAGFLOW_ANYSEARCH_WIRE_RECEIPTS"); path != "" {
				wire := anySearchSavedCatalogWire(t, path, q.RequestID, started, ended)
				requestJSON, _ := json.Marshal(tc.Request)
				var expected map[string]any
				if json.Unmarshal(requestJSON, &expected) != nil {
					t.Fatal("cannot encode the expected catalog request")
				}
				want, _ := json.Marshal(expected)
				got, _ := json.Marshal(wire["request"])
				if wire["method"] != http.MethodPost || wire["url"] != "https://api.anysearch.com/v1/search" || wire["status"] != float64(http.StatusOK) || wire["business_code"] != float64(0) || !bytes.Equal(got, want) {
					t.Fatal("correlated AnySearch wire request did not match the saved catalog selection")
				}
				assertAnySearchObservedResults(t, wire, q.Results, tc.Request.MaxResults)
				h.receipt("catalog-"+tc.Name+"-wire", wire)
			}
			persisted := h.call(http.MethodGet, "/api/v1/agents/"+created.ID+"/sessions/"+session+"?include_reference=true", nil)
			h.receipt("catalog-"+tc.Name+"-saved", json.RawMessage(persisted))
			var history struct {
				Messages []struct {
					Role      string `json:"role"`
					Content   string `json:"content"`
					Reference struct {
						Chunks []anySearchSavedReferenceChunk `json:"chunks"`
					} `json:"reference"`
				} `json:"message"`
			}
			h.decode(persisted, &history)
			found := false
			for _, message := range history.Messages {
				if message.Role != "assistant" || message.Content != answer {
					continue
				}
				found = true
				assertAnySearchSavedReferences(t, q.Results, q.ReferenceIDs, message.Reference.Chunks)
			}
			if !found {
				t.Fatal("saved session lost the exact user-delivered catalog answer")
			}
			t.Logf("saved Canvas delivered catalog request_id=%s results=%d; source scope requires separate review", q.RequestID, len(q.Results))
		})
	}
}

// assertAnySearchObservedResults compares the actual vendor records with the
// delivered records, allowing only the tool's declared count and text limits.
func assertAnySearchObservedResults(t *testing.T, wire map[string]any, delivered []map[string]any, maxResults int) {
	t.Helper()
	body, ok := wire["body"].(map[string]any)
	if !ok {
		t.Fatal("wire receipt has no observed vendor response object")
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatal("observed vendor response has no data object")
	}
	results, ok := data["results"].([]any)
	if !ok || maxResults < 1 || maxResults > 10 {
		t.Fatal("observed results or configured result count are invalid")
	}
	expected := make([]map[string]any, 0, min(len(results), maxResults))
	for _, raw := range results {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatal("observed vendor results contain a non-object record")
		}
		if len(expected) == maxResults {
			continue
		}
		result := maps.Clone(item)
		for _, field := range []string{"content", "snippet"} {
			if text, ok := result[field].(string); ok {
				runes := []rune(strings.TrimSpace(text))
				result[field] = string(runes[:min(len(runes), 1500)])
			}
		}
		expected = append(expected, result)
	}
	want, err := json.Marshal(expected)
	if err != nil {
		t.Fatal("cannot encode observed result comparison")
	}
	got, err := json.Marshal(delivered)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("delivered results differ from the observed vendor data beyond declared count/text limits")
	}
}

type anySearchSavedReferenceChunk struct {
	ID           string `json:"id"`
	ChunkID      string `json:"chunk_id"`
	DocumentID   string `json:"document_id"`
	DocumentName string `json:"document_name"`
	Content      string `json:"content"`
}

func assertAnySearchSavedReferences(t *testing.T, results []map[string]any, ids map[string][2]string, chunks []anySearchSavedReferenceChunk) {
	t.Helper()
	text := func(value any) string {
		valueString, _ := value.(string)
		return strings.TrimSpace(valueString)
	}
	eligible := 0
	var expectedChunks []anySearchSavedReferenceChunk
	for index, result := range results {
		resultURL := text(result["url"])
		u, err := url.ParseRequestURI(resultURL)
		content := text(result["content"])
		if content == "" {
			content = text(result["snippet"])
		}
		content = strings.Join(strings.Fields(content), " ")
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || strings.HasSuffix(u.Host, ":") || content == "" {
			continue
		}
		eligible++
		pair, exists := ids[fmt.Sprint(index)]
		if !exists || pair[0] == "" || pair[1] == "" {
			t.Fatal("an eligible HTTP result lost its document/display reference IDs")
		}
		expectedChunk := anySearchSavedReferenceChunk{ID: pair[1], ChunkID: pair[0], DocumentID: pair[0], DocumentName: text(result["title"]), Content: content}
		expectedChunks = append(expectedChunks, expectedChunk)
		retained := false
		for _, chunk := range chunks {
			if chunk == expectedChunk {
				retained = true
				break
			}
		}
		if !retained {
			t.Fatal("saved reference lost its mapped document identity, title or source content")
		}
	}
	if len(ids) != eligible {
		t.Fatal("reference ID mapping does not exactly cover the independently eligible results")
	}
	for _, chunk := range chunks {
		matched := false
		for _, expected := range expectedChunks {
			matched = matched || chunk == expected
		}
		if !matched {
			t.Fatal("saved references contain a chunk outside the eligible delivered results")
		}
	}
}

func anySearchSavedCatalogWire(t *testing.T, path, requestID string, started, ended time.Time) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		file, err := os.Open(path)
		if err == nil {
			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 4096), 8<<20)
			var matched map[string]any
			requests := 0
			for scanner.Scan() {
				var row map[string]any
				if json.Unmarshal(scanner.Bytes(), &row) != nil {
					continue
				}
				timestamp, _ := row["started_utc"].(string)
				when, parseErr := time.Parse(time.RFC3339Nano, timestamp)
				if parseErr != nil || when.Before(started) || when.After(ended) || row["method"] != http.MethodPost || row["url"] != "https://api.anysearch.com/v1/search" {
					continue
				}
				requests++
				if row["request_id"] == requestID {
					if matched != nil {
						file.Close()
						t.Fatal("multiple wire requests share the delivered request ID")
					}
					matched = row
				}
			}
			readErr := scanner.Err()
			file.Close()
			if readErr != nil {
				t.Fatal("cannot read the complete private wire ledger")
			}
			if matched != nil {
				if requests != 1 {
					t.Fatal("saved catalog run did not make exactly one observed search request")
				}
				return matched
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("no private wire receipt matches the delivered AnySearch request ID")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
