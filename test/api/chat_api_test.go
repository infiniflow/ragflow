//go:build integration

package api

import (
	"fmt"
	"maps"
	"math"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func createRESTChat(t *testing.T, extra apiObject) apiObject {
	t.Helper()
	body := apiObject{"name": uniqueName("go_chat"), "dataset_ids": []string{}}
	maps.Copy(body, extra)
	response := restCall(t, TestConfig, http.MethodPost, "/chats", body, nil)
	// Register cleanup before asserting the response contract.
	if data, ok := response.JSON["data"].(map[string]any); ok {
		if id, ok := data["id"].(string); ok && id != "" {
			cleanupResource(t, TestConfig, "chats", id)
		}
	}
	return object(t, restOK(t, response)["data"])
}

func getRESTChat(t *testing.T, id string) apiObject {
	t.Helper()
	return object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+id, nil, nil))["data"])
}

func chatID(t *testing.T, chat apiObject) string {
	t.Helper()
	id, ok := chat["id"].(string)
	if !ok || id == "" {
		t.Fatal("chat response lacks a nonempty string id")
	}
	return id
}

func assertChatValue(t *testing.T, actual, expected any) {
	t.Helper()
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("value=%#v want=%#v", actual, expected)
	}
}

func chatConfiguration(chat apiObject) apiObject {
	result := maps.Clone(chat)
	for _, key := range []string{"create_time", "create_date", "update_time", "update_date"} {
		delete(result, key)
	}
	return result
}

func TestRESTChatAuthorization(t *testing.T) {
	chat := createRESTChat(t, nil)
	id := chatID(t, chat)
	for _, token := range []string{"", "invalid-api-key"} {
		config := *TestConfig
		config.APIKey = token
		for _, tc := range []struct {
			method, path string
			body         any
		}{
			{http.MethodPost, "/chats", apiObject{"name": uniqueName("unauth")}},
			{http.MethodGet, "/chats", nil},
			{http.MethodGet, "/chats/" + id, nil},
			{http.MethodPut, "/chats/" + id, apiObject{"name": "unauthorized"}},
			{http.MethodPatch, "/chats/" + id, apiObject{"name": "unauthorized"}},
			{http.MethodDelete, "/chats", apiObject{"ids": []string{id}}},
			{http.MethodDelete, "/chats/" + id, nil},
		} {
			t.Run(fmt.Sprintf("%s/%s/%t", tc.method, tc.path, token == ""), func(t *testing.T) {
				result := restCall(t, &config, tc.method, tc.path, tc.body, nil)
				if result.Status != http.StatusUnauthorized {
					t.Fatalf("HTTP=%d want=401", result.Status)
				}
				if result.JSON["code"] == nil || result.JSON["message"] == nil {
					t.Fatal("authorization error lacks code or message")
				}
			})
		}
	}
	if getRESTChat(t, id)["name"] != chat["name"] {
		t.Fatal("unauthorized mutation changed chat")
	}
}

func TestRESTChatCRUD(t *testing.T) {
	chat := createRESTChat(t, nil)
	id := chatID(t, chat)
	other := createRESTChat(t, nil)
	for _, query := range []apiObject{
		{"id": id},
		{"id": id, "keywords": other["name"]},
		{"name": chat["name"], "keywords": other["name"]},
	} {
		data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats", nil, query))["data"])
		chats := array(t, data["chats"])
		if len(chats) != 1 || object(t, chats[0])["id"] != id || data["total"] != float64(1) {
			t.Fatalf("exact filter returned wrong chat or total: %v", data)
		}
	}
	assertChatValue(t, getRESTChat(t, id)["name"], chat["name"])
	for _, method := range []string{http.MethodPut, http.MethodPatch} {
		name := uniqueName("changed")
		result := object(t, restOK(t, restCall(t, TestConfig, method, "/chats/"+id, apiObject{"name": " " + name + " "}, nil))["data"])
		assertChatValue(t, result["name"], name)
		persisted := getRESTChat(t, id)
		assertChatValue(t, persisted["name"], name)
		assertChatValue(t, persisted["dataset_ids"], []any{})
	}
	data := object(t, restOK(t, restCall(t, TestConfig, http.MethodDelete, "/chats", apiObject{"ids": []string{id}}, nil))["data"])
	assertChatValue(t, data["success_count"], float64(1))
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+id, nil, nil), 200, 404)
	list := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats", nil, apiObject{"id": id}))["data"])
	assertChatValue(t, list["chats"], []any{})
	// Deletion must target only the requested chat.
	assertChatValue(t, getRESTChat(t, chatID(t, other))["name"], other["name"])
}

func TestRESTChatNameValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"missing", nil}, {"empty", ""}, {"whitespace", " \t\n"}, {"number", 1},
		{"ascii_overflow", strings.Repeat("a", 256)}, {"utf8_overflow", strings.Repeat("界", 86)},
	} {
		t.Run("create/"+tc.name, func(t *testing.T) {
			restCode(t, restCall(t, TestConfig, http.MethodPost, "/chats", apiObject{"name": tc.value, "dataset_ids": []string{}}, nil), 200, 102)
		})
	}
	asciiPrefix := uniqueName("boundary_ascii")
	boundaryNames := []string{asciiPrefix + strings.Repeat("a", 255-len(asciiPrefix))}
	utf8Prefix := uniqueName("boundary_utf8")
	utf8Padding := 255 - len(utf8Prefix)
	boundaryNames = append(boundaryNames, utf8Prefix+strings.Repeat("界", utf8Padding/len("界"))+strings.Repeat("a", utf8Padding%len("界")))
	for _, name := range boundaryNames {
		t.Run(fmt.Sprintf("boundary/%d_bytes", len(name)), func(t *testing.T) {
			chat := createRESTChat(t, apiObject{"name": name})
			assertChatValue(t, chat["name"], name)
			assertChatValue(t, getRESTChat(t, chatID(t, chat))["name"], name)
		})
	}
	name := uniqueName("duplicate")
	first := createRESTChat(t, apiObject{"name": name})
	second := createRESTChat(t, apiObject{"name": name})
	assertChatValue(t, second["name"], name+"(1)")
	if first["id"] == second["id"] {
		t.Fatal("duplicate create reused an id")
	}
	target := createRESTChat(t, nil)
	id := chatID(t, target)
	for _, method := range []string{http.MethodPut, http.MethodPatch} {
		for _, value := range []any{"", " ", 1, strings.Repeat("a", 256), name, strings.ToUpper(name)} {
			t.Run(fmt.Sprintf("%s/%v", method, value), func(t *testing.T) {
				before := chatConfiguration(getRESTChat(t, id))
				restCode(t, restCall(t, TestConfig, method, "/chats/"+id, apiObject{"name": value}, nil), 200, 102)
				assertChatValue(t, chatConfiguration(getRESTChat(t, id)), before)
			})
		}
	}
}

func TestRESTChatListing(t *testing.T) {
	prefix := uniqueName("listing")
	var ids []string
	for index := range 5 {
		chat := createRESTChat(t, apiObject{"name": fmt.Sprintf("%s_%d", prefix, index)})
		ids = append(ids, chatID(t, chat))
	}
	list := func(t *testing.T, query apiObject) apiObject {
		t.Helper()
		params := apiObject{"keywords": prefix}
		maps.Copy(params, query)
		return object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats", nil, params))["data"])
	}
	for _, tc := range []struct {
		name  string
		query apiObject
		count int
	}{
		{"default", nil, 5}, {"unknown_param", apiObject{"a": "b"}, 5},
		{"page_two", apiObject{"page": 2, "page_size": 2}, 2},
		{"page_three", apiObject{"page": "3", "page_size": 2}, 1},
		{"beyond_end", apiObject{"page": 99, "page_size": 2}, 0},
		{"page_zero", apiObject{"page": 0, "page_size": 2}, 5},
		{"page_negative", apiObject{"page": -1, "page_size": 2}, 5},
		{"page_alpha", apiObject{"page": "a", "page_size": 2}, 5},
		{"size_without_page", apiObject{"page_size": 1}, 5},
		{"size_string_without_page", apiObject{"page_size": "1"}, 5},
		{"size_six_without_page", apiObject{"page_size": 6}, 5},
		{"size_zero", apiObject{"page": 1, "page_size": 0}, 5},
		{"size_negative", apiObject{"page": 1, "page_size": -1}, 5},
		{"size_alpha", apiObject{"page": 1, "page_size": "a"}, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := list(t, tc.query)
			if len(array(t, data["chats"])) != tc.count || data["total"] != float64(5) {
				t.Fatalf("pagination count/total=%v want count=%d total=5", data, tc.count)
			}
		})
	}
	for _, desc := range []any{"false", "False", false, "true", "True", true, "unknown"} {
		t.Run(fmt.Sprintf("name_order/%v", desc), func(t *testing.T) {
			chats := array(t, list(t, apiObject{"orderby": "name", "desc": desc})["chats"])
			wantIDs := slices.Clone(ids)
			if !strings.EqualFold(fmt.Sprint(desc), "false") {
				slices.Reverse(wantIDs)
			}
			for index, chat := range chats {
				assertChatValue(t, object(t, chat)["id"], wantIDs[index])
			}
		})
	}
	for _, order := range []string{"create_time", "update_time"} {
		t.Run(order, func(t *testing.T) {
			chats := array(t, list(t, apiObject{"orderby": order})["chats"])
			for index := 1; index < len(chats); index++ {
				if object(t, chats[index-1])[order].(float64) < object(t, chats[index])[order].(float64) {
					t.Fatalf("%s is not descending", order)
				}
			}
		})
	}
	// Check page membership, not just length.
	first := array(t, list(t, apiObject{"orderby": "name", "desc": false, "page": 1, "page_size": 2})["chats"])
	second := array(t, list(t, apiObject{"orderby": "name", "desc": false, "page": 2, "page_size": 2})["chats"])
	assertChatValue(t, object(t, first[0])["id"], ids[0])
	assertChatValue(t, object(t, second[0])["id"], ids[2])
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/chats", nil, apiObject{"orderby": "unknown"}), 200, 101)
	assertChatValue(t, list(t, apiObject{"keywords": uniqueName("unmatched")})["chats"], []any{})
	assertChatValue(t, list(t, apiObject{"keywords": prefix + "_1"})["total"], float64(1))
	// Empty and omitted keywords are equivalent; no assumption about unrelated tenant resources.
	all := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats", nil, nil))["data"])
	empty := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats", nil, apiObject{"keywords": ""}))["data"])
	assertChatValue(t, empty["total"], all["total"])
}

func TestRESTChatDeletion(t *testing.T) {
	for _, tc := range []struct {
		name       string
		duplicates bool
		missing    string
	}{
		{"valid", false, ""}, {"duplicate_one", true, ""}, {"partial_invalid", false, "invalid_id"},
		{"partial_punctuation", false, "!@#$%^&*()"}, {"duplicates_and_missing", true, "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := chatID(t, createRESTChat(t, nil))
			ids := []string{id}
			if tc.duplicates {
				ids = append(ids, id)
			}
			if tc.missing != "" {
				ids = append(ids, tc.missing)
			}
			data := object(t, restOK(t, restCall(t, TestConfig, http.MethodDelete, "/chats", apiObject{"ids": ids}, nil))["data"])
			assertChatValue(t, data["success_count"], float64(1))
			wantErrors := 0
			if tc.duplicates {
				wantErrors++
			}
			if tc.missing != "" {
				wantErrors++
			}
			if wantErrors > 0 {
				errors := array(t, data["errors"])
				if len(errors) != wantErrors {
					t.Fatalf("errors=%v want count=%d", errors, wantErrors)
				}
				if tc.duplicates && !slices.Contains(errors, any("Duplicate chat ids: "+id)) {
					t.Fatal("missing duplicate-id diagnostic")
				}
				if tc.missing != "" && !slices.Contains(errors, any("Chat("+tc.missing+") not found.")) {
					t.Fatal("missing unknown-id diagnostic")
				}
			}
			restCode(t, restCall(t, TestConfig, http.MethodDelete, "/chats", apiObject{"ids": []string{id}}, nil), 200, 102)
			restCode(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+id, nil, nil), 200, 404)
		})
	}
	for _, ids := range [][]string{{}, {"missing-1", "missing-2"}} {
		result := restCall(t, TestConfig, http.MethodDelete, "/chats", apiObject{"ids": ids}, nil)
		if len(ids) == 0 {
			restOK(t, result)
		} else {
			restCode(t, result, 200, 102)
			for _, id := range ids {
				if !strings.Contains(fmt.Sprint(result.JSON["message"]), "Chat("+id+") not found.") {
					t.Fatalf("missing individual deletion error for %q", id)
				}
			}
		}
	}
	var ids []string
	for range 100 {
		ids = append(ids, chatID(t, createRESTChat(t, nil)))
	}
	data := object(t, restOK(t, restCall(t, TestConfig, http.MethodDelete, "/chats", apiObject{"ids": ids}, nil))["data"])
	assertChatValue(t, data["success_count"], float64(100))
	for _, id := range ids {
		result := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats", nil, apiObject{"id": id}))["data"])
		assertChatValue(t, result["chats"], []any{})
	}
}

func TestRESTChatConcurrentRequests(t *testing.T) {
	prefix := uniqueName("concurrent")
	var ids []string
	for index := range 20 {
		ids = append(ids, chatID(t, createRESTChat(t, apiObject{"name": fmt.Sprintf("%s_%02d", prefix, index)})))
	}
	// Independent reads and distinct resource deletions; shared configuration is immutable.
	t.Run("reads", func(t *testing.T) {
		for index := range 10 {
			t.Run(fmt.Sprint(index), func(t *testing.T) {
				t.Parallel()
				data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats", nil, apiObject{"keywords": prefix}))["data"])
				if len(array(t, data["chats"])) != len(ids) {
					t.Fatal("concurrent list did not return all isolated chats")
				}
			})
		}
	})
	t.Run("deletes", func(t *testing.T) {
		for _, id := range ids {
			t.Run(id, func(t *testing.T) {
				t.Parallel()
				data := object(t, restOK(t, restCall(t, TestConfig, http.MethodDelete, "/chats", apiObject{"ids": []string{id}}, nil))["data"])
				assertChatValue(t, data["success_count"], float64(1))
			})
		}
	})
	data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats", nil, apiObject{"keywords": prefix}))["data"])
	assertChatValue(t, data["chats"], []any{})
}

func TestRESTChatConfigurationMatrices(t *testing.T) {
	type scenario struct {
		name, field string
		value, want any
		reject      bool
	}
	var scenarios []scenario
	for _, field := range []string{"temperature", "top_p", "presence_penalty", "frequency_penalty", "max_token"} {
		values := []any{0, 1, -1, 10, "a"}
		if field == "max_token" {
			values[1] = 1024
		}
		for _, value := range values {
			scenarios = append(scenarios, scenario{"llm/" + field + "/" + fmt.Sprint(value), "llm_setting", apiObject{field: value}, apiObject{field: normalizeChatNumber(value)}, false})
		}
	}
	scenarios = append(scenarios, scenario{"llm/unknown_key", "llm_setting", apiObject{"unknown": "unknown"}, apiObject{"unknown": "unknown"}, false})
	for _, field := range []string{"similarity_threshold", "top_n"} {
		for _, value := range []any{0, 1, -1, 10, "a"} {
			want := normalizeChatNumber(value)
			if value == "a" {
				want = float64(0)
			}
			scenarios = append(scenarios, scenario{field + "/" + fmt.Sprint(value), field, value, want, false})
		}
	}
	for _, value := range []any{0, 1, 2, -9, -1, 10, "a"} {
		valid := value == 0 || value == 1
		scenarios = append(scenarios, scenario{"vector_similarity_weight/" + fmt.Sprint(value), "vector_similarity_weight", value, normalizeChatNumber(value), !valid})
	}
	for _, field := range []string{"empty_response", "prologue", "system"} {
		values := []any{"Hello World", "", "!@#$%^&*()", "中文测试", 123, true, " "}
		if field == "system" {
			values = []any{"Hello World {knowledge}", "{knowledge}", "!@#$%^&*() {knowledge}", "中文测试 {knowledge}", "Hello World", 123, true}
		}
		for _, value := range values {
			scenarios = append(scenarios, scenario{"prompt/" + field + "/" + fmt.Sprint(value), "prompt_config", apiObject{field: value}, apiObject{field: normalizeChatNumber(value)}, false})
		}
	}
	for _, value := range []bool{true, false} {
		scenarios = append(scenarios, scenario{"prompt/quote/" + fmt.Sprint(value), "prompt_config", apiObject{"quote": value}, apiObject{"quote": value}, false})
	}
	scenarios = append(scenarios,
		scenario{"prompt/empty_parameters", "prompt_config", apiObject{"parameters": []any{}}, apiObject{"parameters": []any{}}, false},
		scenario{"prompt/system_empty_parameters", "prompt_config", apiObject{"system": "Hello World", "parameters": []any{}}, apiObject{"system": "Hello World", "parameters": []any{}}, false},
		scenario{"prompt/unknown_key", "prompt_config", apiObject{"unknown": "unknown"}, apiObject{"unknown": "unknown"}, false},
		scenario{"icon", "icon", "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=", "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=", false},
		scenario{"icon/raw", "icon", "raw-avatar-value", "raw-avatar-value", false},
	)
	for _, mode := range []string{"create", http.MethodPut, http.MethodPatch} {
		t.Run(mode, func(t *testing.T) {
			for _, tc := range scenarios {
				t.Run(tc.name, func(t *testing.T) {
					body := apiObject{tc.field: tc.value}
					var persisted apiObject
					if mode == "create" {
						body["name"] = uniqueName("matrix")
						body["dataset_ids"] = []any{}
						if tc.reject {
							restCode(t, restCall(t, TestConfig, http.MethodPost, "/chats", body, nil), 200, 102)
							return
						}
						persisted = createRESTChat(t, body)
						persisted = getRESTChat(t, chatID(t, persisted))
					} else {
						chat := createRESTChat(t, nil)
						id := chatID(t, chat)
						before := chatConfiguration(getRESTChat(t, id))
						result := restCall(t, TestConfig, mode, "/chats/"+id, body, nil)
						// Creation converts invalid numeric strings to zero; updates
						// forward them to typed SQL columns and reject them.
						reject := tc.reject || (tc.value == "a" && (tc.field == "top_n" || tc.field == "similarity_threshold"))
						if reject {
							restCode(t, result, 200, 102)
							assertChatValue(t, chatConfiguration(getRESTChat(t, id)), before)
							return
						}
						restOK(t, result)
						persisted = getRESTChat(t, id)
					}
					if tc.field == "vector_similarity_weight" {
						if math.Abs(persisted["keywords_similarity_weight"].(float64)-(1-tc.want.(float64))) > 1e-9 {
							t.Fatal("public keyword weight does not complement supplied vector weight")
						}
						if _, exists := persisted["vector_similarity_weight"]; exists {
							t.Fatal("internal vector weight leaked in public response")
						}
					} else if expected, ok := tc.want.(map[string]any); ok && tc.field == "prompt_config" {
						actual := object(t, persisted[tc.field])
						for key, value := range expected {
							assertChatValue(t, actual[key], value)
						}
					} else {
						assertChatValue(t, persisted[tc.field], tc.want)
					}
				})
			}
		})
	}
}

func normalizeChatNumber(value any) any {
	if integer, ok := value.(int); ok {
		return float64(integer)
	}
	return value
}

func TestRESTChatDefaultsAndUpdateSemantics(t *testing.T) {
	chat := createRESTChat(t, nil)
	id := chatID(t, chat)
	persisted := getRESTChat(t, id)
	for key, want := range (apiObject{"similarity_threshold": 0.1, "keywords_similarity_weight": 0.7, "top_n": float64(6), "rerank_id": ""}) {
		assertChatValue(t, persisted[key], want)
	}
	prompt := object(t, persisted["prompt_config"])
	for key, want := range (apiObject{
		"system": "", "prologue": "Hi! I'm your assistant. What can I do for you?", "quote": true,
		"empty_response": "Sorry! No relevant content was found in the knowledge base!",
		"parameters":     []any{apiObject{"key": "knowledge", "optional": false}},
	}) {
		assertChatValue(t, prompt[key], want)
	}
	// llm_id is a composite model name; tenant_llm_id is the configured row ID.
	// Their representations differ and neither is fixed to the CI provider.
	assertChatValue(t, persisted["llm_id"], chat["llm_id"])
	restOK(t, restCall(t, TestConfig, http.MethodPatch, "/chats/"+id, apiObject{"prompt_config": apiObject{"quote": false}, "llm_setting": apiObject{"temperature": 0.2}}, nil))
	restOK(t, restCall(t, TestConfig, http.MethodPatch, "/chats/"+id, apiObject{"prompt_config": apiObject{"prologue": "updated"}, "llm_setting": apiObject{"top_p": 0.4}}, nil))
	persisted = getRESTChat(t, id)
	assertChatValue(t, object(t, persisted["prompt_config"])["quote"], false)
	assertChatValue(t, object(t, persisted["prompt_config"])["prologue"], "updated")
	assertChatValue(t, persisted["llm_setting"], apiObject{"temperature": 0.2, "top_p": 0.4})
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/chats/"+id, apiObject{"prompt_config": apiObject{"system": "replacement"}, "llm_setting": apiObject{"temperature": 0.6}}, nil))
	persisted = getRESTChat(t, id)
	prompt = object(t, persisted["prompt_config"])
	assertChatValue(t, prompt["system"], "replacement")
	if _, exists := prompt["quote"]; exists {
		t.Fatal("PUT merged the previous prompt instead of replacing it")
	}
	assertChatValue(t, persisted["llm_setting"], apiObject{"temperature": 0.6})
	// Unknown top-level fields are ignored; unrelated persisted configuration survives.
	before := chatConfiguration(persisted)
	restOK(t, restCall(t, TestConfig, http.MethodPatch, "/chats/"+id, apiObject{"unknown": "unknown"}, nil))
	assertChatValue(t, chatConfiguration(getRESTChat(t, id)), before)
	restOK(t, restCall(t, TestConfig, http.MethodPatch, "/chats/"+id, apiObject{"prompt_config": apiObject{"system": "No required placeholder", "parameters": []any{apiObject{"key": "knowledge", "optional": false}}}}, nil))
	prompt = object(t, getRESTChat(t, id)["prompt_config"])
	assertChatValue(t, prompt["system"], "No required placeholder")
	assertChatValue(t, prompt["parameters"], []any{apiObject{"key": "knowledge", "optional": false}})
	t.Run("explicit_configured_model", func(t *testing.T) {
		model, ok := chat["llm_id"].(string)
		if !ok || model == "" {
			t.Skip("tenant has no default chat model to exercise explicit configured model resolution")
		}
		created := createRESTChat(t, apiObject{"llm_id": model})
		assertChatValue(t, getRESTChat(t, chatID(t, created))["llm_id"], model)
		for _, method := range []string{http.MethodPut, http.MethodPatch} {
			restOK(t, restCall(t, TestConfig, method, "/chats/"+id, apiObject{"llm_id": model}, nil))
			assertChatValue(t, getRESTChat(t, id)["llm_id"], model)
		}
	})
}

func TestRESTChatRejectedMutations(t *testing.T) {
	chat := createRESTChat(t, nil)
	id := chatID(t, chat)
	duplicateParameters := apiObject{"parameters": []any{apiObject{"key": "knowledge"}, apiObject{"key": "knowledge"}}}
	for _, tc := range []struct {
		name string
		body apiObject
	}{
		{"unknown_llm", apiObject{"llm_id": uniqueName("missing_model")}},
		{"unknown_rerank", apiObject{"rerank_id": uniqueName("missing_rerank")}},
		{"duplicate_parameters", apiObject{"prompt_config": duplicateParameters}},
		{"invalid_prompt", apiObject{"prompt_config": "invalid"}},
		{"invalid_dataset_type", apiObject{"dataset_ids": "invalid"}},
		{"unknown_dataset", apiObject{"dataset_ids": []string{uniqueName("missing_dataset")}}},
		{"invalid_metadata", apiObject{"meta_data_filter": "invalid"}},
		{"mismatched_weights", apiObject{"keywords_similarity_weight": 0.7, "vector_similarity_weight": 0.7}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			createBody := maps.Clone(tc.body)
			createBody["name"] = uniqueName("rejected")
			restCode(t, restCall(t, TestConfig, http.MethodPost, "/chats", createBody, nil), 200, 102)
			for _, method := range []string{http.MethodPut, http.MethodPatch} {
				before := chatConfiguration(getRESTChat(t, id))
				result := restCall(t, TestConfig, method, "/chats/"+id, tc.body, nil)
				// Dataset authorization is normalized to a hidden 404 on update.
				if tc.name == "unknown_dataset" {
					restCode(t, result, 200, 404)
				} else {
					restCode(t, result, 200, 102)
				}
				assertChatValue(t, chatConfiguration(getRESTChat(t, id)), before)
			}
		})
	}
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		path := "/chats"
		body := apiObject{"name": uniqueName("tenant_override"), "tenant_id": "forged"}
		if method == http.MethodPut {
			path += "/" + id
		}
		restCode(t, restCall(t, TestConfig, method, path, body, nil), 200, 102)
	}
	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodGet, http.MethodDelete} {
		restCode(t, restCall(t, TestConfig, method, "/chats/"+uniqueName("missing"), apiObject{"name": "missing"}, nil), 200, 404)
	}
}

func TestRESTChatDatasetAssociations(t *testing.T) {
	dataset := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, "/datasets", apiObject{"name": uniqueName("chat_dataset"), "parse_type": 1, "parser_id": "naive"}, nil))["data"])
	id, ok := dataset["id"].(string)
	if !ok || id == "" {
		t.Fatal("dataset creation lacks id")
	}
	t.Cleanup(func() {
		restCall(t, TestConfig, http.MethodDelete, "/datasets", apiObject{"ids": []string{id}}, nil)
	})
	chat := createRESTChat(t, nil)
	targetID := chatID(t, chat)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		t.Run(method+"/unparsed", func(t *testing.T) {
			body := apiObject{"name": uniqueName("unparsed"), "dataset_ids": []string{id}}
			path := "/chats"
			if method != http.MethodPost {
				path += "/" + targetID
			}
			before := chatConfiguration(getRESTChat(t, targetID))
			result := restCall(t, TestConfig, method, path, body, nil)
			restCode(t, result, 200, 102)
			if !strings.Contains(fmt.Sprint(result.JSON["message"]), "doesn't own parsed file") {
				t.Fatal("unparsed dataset rejection has incorrect explanation")
			}
			assertChatValue(t, chatConfiguration(getRESTChat(t, targetID)), before)
		})
	}
	t.Run("parsed_dataset", func(t *testing.T) {
		parsedID := os.Getenv("RAGFLOW_TEST_PARSED_DATASET_ID")
		if parsedID == "" {
			t.Skip("requires RAGFLOW_TEST_PARSED_DATASET_ID owned by this account with parsed chunks; ingestion is outside this suite")
		}
		created := createRESTChat(t, apiObject{"dataset_ids": []string{parsedID}})
		createdID := chatID(t, created)
		assertChatValue(t, getRESTChat(t, createdID)["dataset_ids"], []any{parsedID})
		system := object(t, getRESTChat(t, createdID)["prompt_config"])["system"]
		if text, ok := system.(string); !ok || !strings.Contains(text, "{knowledge}") {
			t.Fatal("dataset-bound chat lacks default knowledge prompt")
		}
		for _, method := range []string{http.MethodPut, http.MethodPatch} {
			for _, value := range []any{[]any{}, []string{parsedID}, nil} {
				restOK(t, restCall(t, TestConfig, method, "/chats/"+createdID, apiObject{"dataset_ids": value}, nil))
				want := []any{}
				if ids, ok := value.([]string); ok {
					want = []any{ids[0]}
				}
				assertChatValue(t, getRESTChat(t, createdID)["dataset_ids"], want)
			}
		}
	})
	t.Run("list_after_dataset_deletion", func(t *testing.T) {
		disposableID := os.Getenv("RAGFLOW_TEST_DISPOSABLE_PARSED_DATASET_ID")
		if disposableID == "" {
			t.Skip("requires RAGFLOW_TEST_DISPOSABLE_PARSED_DATASET_ID owned by this account; this fixture is explicitly deleted")
		}
		cleanupResource(t, TestConfig, "datasets", disposableID)
		created := createRESTChat(t, apiObject{"dataset_ids": []string{disposableID}})
		createdID := chatID(t, created)
		restOK(t, restCall(t, TestConfig, http.MethodDelete, "/datasets", apiObject{"ids": []string{disposableID}}, nil))
		data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats", nil, apiObject{"id": createdID}))["data"])
		chats := array(t, data["chats"])
		if len(chats) != 1 || object(t, chats[0])["id"] != createdID {
			t.Fatal("deleting associated dataset removed or hid its chat")
		}
		assertChatValue(t, getRESTChat(t, createdID)["dataset_ids"], []any{})
	})
}

func TestRESTChatOtherAccount(t *testing.T) {
	key := os.Getenv("RAGFLOW_TEST_OTHER_API_KEY")
	if key == "" {
		t.Skip("requires RAGFLOW_TEST_OTHER_API_KEY for a separate account without shared access")
	}
	chat := createRESTChat(t, nil)
	id := chatID(t, chat)
	config := *TestConfig
	config.APIKey = key
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		result := restCall(t, &config, method, "/chats/"+id, apiObject{"name": "forbidden"}, nil)
		code := 403
		if method == http.MethodGet || method == http.MethodDelete {
			code = 404
		}
		restCode(t, result, 200, code)
	}
	data := object(t, restOK(t, restCall(t, &config, http.MethodGet, "/chats", nil, apiObject{"id": id}))["data"])
	assertChatValue(t, data["chats"], []any{})
	assertChatValue(t, getRESTChat(t, id)["name"], chat["name"])
}
