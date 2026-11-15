//go:build integration

package api

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestDatasetCreateValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fields   apiObject
		code     int
		fragment string
	}{
		{"missing_name", apiObject{"name": nil}, 102, "required"},
		{"empty_name", apiObject{"name": ""}, 102, "required"},
		{"blank_name", apiObject{"name": " \t "}, 102, "empty"},
		{"long_name", apiObject{"name": strings.Repeat("a", 129)}, 102, "128"},
		{"number_name", apiObject{"name": 42}, 102, "cannot unmarshal"},
		{"missing_mode", apiObject{"parse_type": nil}, 102, "parse_type"},
		{"invalid_mode", apiObject{"parse_type": 3}, 102, "parse_type"},
		{"empty_parser", apiObject{"parser_id": ""}, 102, "parser_id"},
		{"unknown_parser", apiObject{"parser_id": "unknown"}, 102, "input should be"},
		{"typed_parser", apiObject{"parser_id": []string{}}, 102, "cannot unmarshal"},
		{"empty_permission", apiObject{"permission": ""}, 102, "'me' or 'team'"},
		{"invalid_permission", apiObject{"permission": "ME"}, 102, "'me' or 'team'"},
		{"typed_permission", apiObject{"permission": []string{}}, 102, "cannot unmarshal"},
		{"invalid_language", apiObject{"language": " "}, 102, "character"},
		{"typed_config", apiObject{"parser_config": []string{}}, 102, "cannot unmarshal"},
		{"config_size", apiObject{"parser_config": apiObject{"GeneralChunker:bad": apiObject{"delimiter": strings.Repeat("x", 65536)}}}, 101, "size limit"},
		{"extra_field", apiObject{"unknown_field": true}, 101, "Extra inputs"},
		{"server_id", apiObject{"id": "injected"}, 101, "Extra inputs"},
		{"pagerank_creation", apiObject{"pagerank": 50}, 101, "Extra inputs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := uniqueName("invalid_create")
			fields := apiObject{"name": name, "parse_type": 1, "parser_id": "general"}
			for key, value := range tc.fields {
				fields[key] = value
			}
			result := restCall(t, TestConfig, http.MethodPost, "/datasets", fields, nil)
			if data, ok := result.JSON["data"].(apiObject); ok {
				if id, ok := data["id"].(string); ok && id != "" {
					cleanupResource(t, TestConfig, "datasets", id)
				}
			}
			restCode(t, result, 200, tc.code)
			if message, _ := result.JSON["message"].(string); !strings.Contains(strings.ToLower(message), strings.ToLower(tc.fragment)) {
				t.Errorf("message=%q should contain %q", message, tc.fragment)
			}
			// Failed validation must not have silently created the original fixture.
			listed := restOK(t, restCall(t, TestConfig, http.MethodGet, "/datasets", nil, apiObject{"name": name}))
			if len(array(t, listed["data"])) != 0 {
				t.Fatal("failed create persisted a dataset")
			}
		})
	}
}

func TestDatasetCreateDefaultsDeduplicationAndBoundaries(t *testing.T) {
	name := uniqueName("case")
	first := createRESTDataset(t, apiObject{"name": strings.ToUpper(name)})
	second := createRESTDataset(t, apiObject{"name": strings.ToLower(name)})
	if second["name"] != strings.ToLower(name)+"(1)" || first["id"] == second["id"] {
		t.Fatalf("case-insensitive deduplication failed: %v %v", first, second)
	}
	boundary := uniqueName("boundary")
	boundary += strings.Repeat("x", 128-len(boundary))
	createRESTDataset(t, apiObject{"name": boundary})
	for _, tc := range []struct {
		name   string
		fields apiObject
	}{{"unset", nil}, {"empty", apiObject{"parser_config": apiObject{}}}, {"null", apiObject{"parser_config": nil, "embedding_model": nil}}} {
		t.Run(tc.name, func(t *testing.T) {
			data := createRESTDataset(t, tc.fields)
			if data["permission"] != "me" || data["parser_id"] != "general" {
				t.Fatalf("unexpected defaults: %v", data)
			}
			requirePipelineParserConfig(t, data["parser_config"], "GeneralChunker:")
			if embedding, ok := data["embedding_model"].(string); !ok || strings.TrimSpace(embedding) == "" {
				t.Fatalf("missing configured default embedding: %v", data["embedding_model"])
			}
		})
	}
}

func TestDatasetEmbeddingValidationAndRoundtrip(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	id := dataset["id"].(string)
	for _, model := range []string{"", " ", "missing-provider", "@Builtin", "model@", " @Builtin", "model@ ", "not-a-real-model@not-a-real-provider"} {
		t.Run(model, func(t *testing.T) {
			fields := apiObject{"embedding_model": model}
			restCode(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, fields, nil), 200, 102)
			requireDatasetFields(t, id, apiObject{"embedding_model": dataset["embedding_model"]})
			fields["name"] = uniqueName("bad_model")
			fields["parse_type"] = 1
			fields["parser_id"] = "general"
			result := restCall(t, TestConfig, http.MethodPost, "/datasets", fields, nil)
			if data, ok := result.JSON["data"].(apiObject); ok {
				if createdID, ok := data["id"].(string); ok && createdID != "" {
					cleanupResource(t, TestConfig, "datasets", createdID)
				}
			}
			restCode(t, result, 200, 102)
		})
	}
	// Use the tenant's configured model, rather than a hardcoded provider that may
	// not be authorized in the environment running these integration tests.
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"embedding_model": dataset["embedding_model"]}, nil))
	requireDatasetFields(t, id, apiObject{"embedding_model": dataset["embedding_model"]})
	createRESTDataset(t, apiObject{"embedding_model": dataset["embedding_model"]})
}

func TestDatasetUpdateValidationPreservesState(t *testing.T) {
	data := createRESTDataset(t, nil)
	id := data["id"].(string)
	for _, tc := range []struct {
		name   string
		fields apiObject
		code   int
	}{
		{"empty", apiObject{}, 102},
		{"empty_name", apiObject{"name": ""}, 102},
		{"blank_name", apiObject{"name": " \t "}, 102},
		{"long_name", apiObject{"name": strings.Repeat("x", 129)}, 102},
		{"typed_name", apiObject{"name": 42}, 102},
		{"description_limit", apiObject{"description": strings.Repeat("x", 65536)}, 102},
		{"typed_description", apiObject{"description": []string{}}, 102},
		{"avatar_limit", apiObject{"avatar": strings.Repeat("x", 65536)}, 102},
		{"invalid_avatar", apiObject{"avatar": "not-an-image"}, 102},
		{"avatar_mime", apiObject{"avatar": "data:image/gif;base64,eA=="}, 102},
		{"permission", apiObject{"permission": "unknown"}, 102},
		{"permission_type", apiObject{"permission": []string{}}, 102},
		{"parser_empty", apiObject{"parse_type": 1, "parser_id": ""}, 102},
		{"parser_unknown", apiObject{"parse_type": 1, "parser_id": "unknown"}, 102},
		{"parser_missing_mode", apiObject{"parser_id": "qa"}, 102},
		{"pagerank_negative", apiObject{"pagerank": -1}, 102},
		{"pagerank_large", apiObject{"pagerank": 101}, 102},
		{"pagerank_type", apiObject{"pagerank": "50"}, 102},
		{"connectors_invalid", apiObject{"connectors": []any{apiObject{"id": ""}}}, 102},
		{"config_type", apiObject{"parser_config": "invalid"}, 102},
		{"config_size", apiObject{"parser_config": apiObject{"GeneralChunker:invalid": apiObject{"delimiter": strings.Repeat("x", 65536)}}}, 101},
		{"config_ext", apiObject{"parser_config": apiObject{"ext": apiObject{}}}, 101},
		{"unsupported", apiObject{"unknown_field": true}, 101},
		{"ownership", apiObject{"tenant_id": "other"}, 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restCode(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, tc.fields, nil), 200, tc.code)
			actual := getRESTDataset(t, id)
			for _, key := range []string{"name", "description", "avatar", "permission", "parser_id", "parser_config", "embedding_model", "pagerank", "language", "connectors"} {
				if !reflect.DeepEqual(data[key], actual[key]) {
					t.Errorf("failed update changed %s: before=%v after=%v", key, data[key], actual[key])
				}
			}
		})
	}
	for _, tc := range []struct {
		id   string
		code int
	}{{"not_uuid", 101}, {"00000000000040008000000000000000", 404}} {
		restCode(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+tc.id, apiObject{"description": "missing"}, nil), 200, tc.code)
	}
}

func TestDatasetPermissionAndPartialUpdatePreservation(t *testing.T) {
	data := createRESTDataset(t, apiObject{"permission": "team"})
	id := data["id"].(string)
	for _, permission := range []string{"me", "team"} {
		restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"permission": permission}, nil))
		requireDatasetFields(t, id, apiObject{"permission": permission})
	}
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"description": "preserve me"}, nil))
	expected := getRESTDataset(t, id)
	newName := uniqueName("partial")
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"name": newName}, nil))
	actual := getRESTDataset(t, id)
	for _, key := range []string{"description", "avatar", "permission", "parser_id", "parser_config", "embedding_model", "pagerank", "language", "connectors"} {
		if !reflect.DeepEqual(expected[key], actual[key]) {
			t.Errorf("partial update changed unprovided %s", key)
		}
	}
	if actual["name"] != newName {
		t.Errorf("name=%v want=%s", actual["name"], newName)
	}
}

func TestDatasetChunkMethodMatrix(t *testing.T) {
	for _, method := range []string{"general", "naive", "book", "email", "laws", "manual", "one", "paper", "picture", "presentation", "qa", "table"} {
		t.Run(method, func(t *testing.T) {
			data := createRESTDataset(t, apiObject{"parser_id": method})
			id := data["id"].(string)
			expected := method
			if method == "naive" {
				expected = "general"
			}
			if data["parser_id"] != expected {
				t.Fatalf("parser=%v want=%s", data["parser_id"], expected)
			}
			restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"parse_type": 1, "parser_id": "general"}, nil))
			restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"parse_type": 1, "parser_id": method}, nil))
			requireDatasetFields(t, id, apiObject{"parser_id": expected})
		})
	}
}

func TestDatasetMalformedRequestPreservesState(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	id := dataset["id"].(string)
	for _, tc := range []struct {
		name, body, contentType string
	}{
		{"empty", "", "application/json"},
		{"truncated", `{"name":`, "application/json"},
		{"array", `[]`, "application/json"},
		{"scalar", `"name"`, "application/json"},
		{"null", `null`, "application/json"},
		{"wrong_content_type", `{"name":"invalid"}`, "text/plain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, target := range []struct{ method, path string }{
				{http.MethodPost, "/datasets"},
				{http.MethodPut, "/datasets/" + id},
				{http.MethodDelete, "/datasets"},
			} {
				result := rawRESTCall(t, TestConfig, target.method, target.path, []byte(tc.body), tc.contentType)
				if data, ok := result.JSON["data"].(apiObject); ok && target.method == http.MethodPost {
					if createdID, ok := data["id"].(string); ok && createdID != "" {
						cleanupResource(t, TestConfig, "datasets", createdID)
					}
				}
				restCode(t, result, 200, 101)
				requireDatasetFields(t, id, apiObject{"name": dataset["name"], "parser_config": dataset["parser_config"]})
			}
		})
	}
}

func TestDatasetParserNormalization(t *testing.T) {
	data := createRESTDataset(t, apiObject{"language": "Klingon", "parser_config": apiObject{"chunk_token_num": 19, "GeneralChunker:unknown": "ignored"}})
	id := data["id"].(string)
	requireDatasetFields(t, id, apiObject{"language": "Klingon"})
	config := requirePipelineParserConfig(t, data["parser_config"], "GeneralChunker:")
	if _, exists := config["GeneralChunker:unknown"]; exists {
		t.Fatalf("unknown component survived pipeline normalization: %v", config)
	}
}

func TestDatasetPagerankBoundariesAndReset(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	id := dataset["id"].(string)
	status := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/system/status", nil, nil))["data"])
	engine := object(t, status["doc_engine"])["type"]
	if engine == "" || engine == nil {
		t.Fatal("system status did not identify the document engine")
	}
	for _, value := range []int{0, 100, 50, 0} {
		result := restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"pagerank": value}, nil)
		if engine == "infinity" && value != 0 {
			restCode(t, result, 200, 102)
			requireDatasetFields(t, id, apiObject{"pagerank": float64(0)})
		} else {
			restOK(t, result)
			requireDatasetFields(t, id, apiObject{"pagerank": float64(value)})
		}
	}
}

func TestDatasetEmbeddingUpdatePreservesExistingChunks(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	id := dataset["id"].(string)
	document := object(t, uploadRESTDocuments(t, TestConfig, id, []documentUpload{{Name: "embedding.txt", Content: []byte("existing embedding chunk")}})[0])
	path := "/datasets/" + id + "/documents/" + document["id"].(string) + "/chunks"
	content := "Dataset embedding roundtrip must preserve this existing chunk."
	added := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, path, apiObject{"content": content}, nil))["data"])
	chunk := object(t, added["chunk"])
	chunkID := chunk["id"].(string)
	persisted := getRESTDataset(t, id)
	if count, ok := persisted["chunk_count"].(float64); !ok || count < 1 {
		t.Fatalf("fixture has no persisted chunks: %v", persisted)
	}
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"embedding_model": dataset["embedding_model"]}, nil))
	requireDatasetFields(t, id, apiObject{"embedding_model": dataset["embedding_model"], "chunk_count": persisted["chunk_count"]})
	data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, path, nil, nil))["data"])
	chunks := array(t, data["chunks"])
	if len(chunks) != 1 {
		t.Fatalf("embedding update changed document chunks: %v", chunks)
	}
	actual := object(t, chunks[0])
	if actual["chunk_id"] != chunkID || actual["content_with_weight"] != content {
		t.Fatalf("embedding update corrupted existing chunk: %v", actual)
	}
}
