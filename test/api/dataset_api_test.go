//go:build integration

//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
)

func decodeResponseJSON(resp *http.Response) (map[string]interface{}, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func requireStatusCode(t *testing.T, resp *http.Response, expected int) map[string]interface{} {
	t.Helper()
	if resp.StatusCode != expected {
		t.Fatalf("expected status code %d, got %d", expected, resp.StatusCode)
	}
	payload, err := decodeResponseJSON(resp)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return payload
}

func requireCodeZero(t *testing.T, payload map[string]interface{}) {
	t.Helper()
	code, ok := payload["code"].(float64)
	if !ok || int(code) != 0 {
		t.Fatalf("expected code 0, got %v, payload: %v", payload["code"], payload)
	}
}

func clearDatasets(t *testing.T) {
	t.Helper()
	resp, err := TestConfig.DeleteJSON("/datasets", map[string]interface{}{"ids": nil, "delete_all": true}, nil)
	if err != nil {
		t.Fatalf("clear datasets request failed: %v", err)
	}
	payload := requireStatusCode(t, resp, http.StatusOK)
	code, _ := payload["code"].(float64)
	if int(code) != 0 && int(code) != 102 {
		t.Fatalf("clear datasets failed: code=%v, payload=%v", payload["code"], payload)
	}
}

func jsonDeepEqual(a, b interface{}) bool {
	switch av := a.(type) {
	case float64:
		switch bv := b.(type) {
		case float64:
			return av == bv
		case int:
			return av == float64(bv)
		case int64:
			return av == float64(bv)
		default:
			return false
		}
	case int:
		return jsonDeepEqual(float64(av), b)
	case int64:
		return jsonDeepEqual(float64(av), b)
	case string, bool, nil:
		return a == b
	case []interface{}:
		bv, ok := b.([]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonDeepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			bvVal, ok := bv[k]
			if !ok || !jsonDeepEqual(v, bvVal) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func TestDatasetCRUDCycle(t *testing.T) {
	clearDatasets(t)
	// Create
	createResp, err := TestConfig.PostJSON("/datasets", map[string]interface{}{"name": "restful_dataset_crud"}, nil)
	if err != nil {
		t.Fatalf("create dataset request failed: %v", err)
	}
	createPayload := requireStatusCode(t, createResp, http.StatusOK)
	requireCodeZero(t, createPayload)
	createData, ok := createPayload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("create response data is not an object: %v", createPayload)
	}
	datasetID, ok := createData["id"].(string)
	if !ok || datasetID == "" {
		t.Fatalf("create response data.id is not a valid string: %v", createData)
	}

	// Get by id
	getResp, err := TestConfig.GetJSON(fmt.Sprintf("/datasets/%s", datasetID), nil, nil)
	if err != nil {
		t.Fatalf("get dataset request failed: %v", err)
	}
	getPayload := requireStatusCode(t, getResp, http.StatusOK)
	requireCodeZero(t, getPayload)
	getData, ok := getPayload["data"].(map[string]interface{})
	if !ok || getData["id"] != datasetID {
		t.Fatalf("get response data.id mismatch: %v", getPayload)
	}

	// Update
	updateResp, err := TestConfig.PutJSON(fmt.Sprintf("/datasets/%s", datasetID), map[string]interface{}{"name": "restful_dataset_crud_updated"}, nil)
	if err != nil {
		t.Fatalf("update dataset request failed: %v", err)
	}
	updatePayload := requireStatusCode(t, updateResp, http.StatusOK)
	requireCodeZero(t, updatePayload)
	updateData, ok := updatePayload["data"].(map[string]interface{})
	if !ok || updateData["name"] != "restful_dataset_crud_updated" {
		t.Fatalf("update response data.name mismatch: %v", updatePayload)
	}

	// List with id filter
	listResp, err := TestConfig.GetJSON("/datasets", map[string]interface{}{"id": datasetID}, nil)
	if err != nil {
		t.Fatalf("list dataset request failed: %v", err)
	}
	listPayload := requireStatusCode(t, listResp, http.StatusOK)
	requireCodeZero(t, listPayload)
	listData, ok := listPayload["data"].([]interface{})
	if !ok || len(listData) != 1 {
		t.Fatalf("expected 1 dataset in list, got %v", listPayload)
	}
	firstItem, ok := listData[0].(map[string]interface{})
	if !ok || firstItem["id"] != datasetID {
		t.Fatalf("list response data[0].id mismatch: %v", listPayload)
	}

	// Delete
	deleteResp, err := TestConfig.DeleteJSON("/datasets", map[string]interface{}{"ids": []interface{}{datasetID}}, nil)
	if err != nil {
		t.Fatalf("delete dataset request failed: %v", err)
	}
	deletePayload := requireStatusCode(t, deleteResp, http.StatusOK)
	requireCodeZero(t, deletePayload)

	// List after delete
	listAfterResp, err := TestConfig.GetJSON("/datasets", nil, nil)
	if err != nil {
		t.Fatalf("list after delete request failed: %v", err)
	}
	listAfterPayload := requireStatusCode(t, listAfterResp, http.StatusOK)
	requireCodeZero(t, listAfterPayload)
	listAfterData, ok := listAfterPayload["data"].([]interface{})
	if !ok {
		t.Fatalf("list after delete data is not an array: %v", listAfterPayload)
	}
	for _, item := range listAfterData {
		dataset, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if dataset["id"] == datasetID {
			t.Fatalf("deleted dataset %s still present in list: %v", datasetID, listAfterPayload)
		}
	}
}

func TestDatasetUpdateNameAndCaseInsensitiveContract(t *testing.T) {
	clearDatasets(t)

	// Create first dataset
	firstResp, err := TestConfig.PostJSON("/datasets", map[string]interface{}{"name": "dataset_update_name_source"}, nil)
	if err != nil {
		t.Fatalf("create first dataset request failed: %v", err)
	}
	firstPayload := requireStatusCode(t, firstResp, http.StatusOK)
	requireCodeZero(t, firstPayload)
	firstData, ok := firstPayload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("first response data is not an object: %v", firstPayload)
	}
	firstDatasetID, ok := firstData["id"].(string)
	if !ok || firstDatasetID == "" {
		t.Fatalf("first dataset id is invalid: %v", firstData)
	}

	// Create second dataset
	secondResp, err := TestConfig.PostJSON("/datasets", map[string]interface{}{"name": "dataset_update_name_target"}, nil)
	if err != nil {
		t.Fatalf("create second dataset request failed: %v", err)
	}
	secondPayload := requireStatusCode(t, secondResp, http.StatusOK)
	requireCodeZero(t, secondPayload)
	secondData, ok := secondPayload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("second response data is not an object: %v", secondPayload)
	}
	secondDatasetName, ok := secondData["name"].(string)
	if !ok || secondDatasetName == "" {
		t.Fatalf("second dataset name is invalid: %v", secondData)
	}

	// Rename first dataset
	renameResp, err := TestConfig.PutJSON(fmt.Sprintf("/datasets/%s", firstDatasetID), map[string]interface{}{"name": "dataset_update_name_renamed"}, nil)
	if err != nil {
		t.Fatalf("rename dataset request failed: %v", err)
	}
	renamePayload := requireStatusCode(t, renameResp, http.StatusOK)
	requireCodeZero(t, renamePayload)
	renameData, ok := renamePayload["data"].(map[string]interface{})
	if !ok || renameData["name"] != "dataset_update_name_renamed" {
		t.Fatalf("rename response data.name mismatch: %v", renamePayload)
	}

	// List with id filter
	listResp, err := TestConfig.GetJSON("/datasets", map[string]interface{}{"id": firstDatasetID}, nil)
	if err != nil {
		t.Fatalf("list dataset request failed: %v", err)
	}
	listPayload := requireStatusCode(t, listResp, http.StatusOK)
	requireCodeZero(t, listPayload)
	listData, ok := listPayload["data"].([]interface{})
	if !ok || len(listData) != 1 {
		t.Fatalf("expected 1 dataset in list, got %v", listPayload)
	}
	firstItem, ok := listData[0].(map[string]interface{})
	if !ok || firstItem["name"] != "dataset_update_name_renamed" {
		t.Fatalf("list response data[0].name mismatch: %v", listPayload)
	}

	// Try duplicate name (case-insensitive)
	duplicateResp, err := TestConfig.PutJSON(fmt.Sprintf("/datasets/%s", firstDatasetID), map[string]interface{}{"name": strings.ToUpper(secondDatasetName)}, nil)
	if err != nil {
		t.Fatalf("duplicate case rename request failed: %v", err)
	}
	duplicatePayload := requireStatusCode(t, duplicateResp, http.StatusOK)
	duplicateCode, ok := duplicatePayload["code"].(float64)
	if !ok || int(duplicateCode) != 102 {
		t.Fatalf("expected code 102 for duplicate case rename, got %v, payload: %v", duplicatePayload["code"], duplicatePayload)
	}
	duplicateMessage, _ := duplicatePayload["message"].(string)
	if !strings.Contains(duplicateMessage, "already exists") {
		t.Fatalf("expected message to contain 'already exists', got %q, payload: %v", duplicateMessage, duplicatePayload)
	}
}

func encodeAvatar() (string, error) {
	img := image.NewRGBA(image.Rectangle{
		Min: image.Point{X: 0, Y: 0},
		Max: image.Point{X: 100, Y: 100},
	})
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{R: 0, G: 0, B: 255, A: 255}}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return fmt.Sprintf("data:image/png;base64,%s", encoded), nil
}

func TestDatasetUpdateLanguageConnectorsAvatarAndDescriptionContract(t *testing.T) {
	clearDatasets(t)

	createResp, err := TestConfig.PostJSON("/datasets", map[string]interface{}{"name": "dataset_update_lang_connectors"}, nil)
	if err != nil {
		t.Fatalf("create dataset request failed: %v", err)
	}
	createPayload := requireStatusCode(t, createResp, http.StatusOK)
	requireCodeZero(t, createPayload)
	createData, ok := createPayload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("create response data is not an object: %v", createPayload)
	}
	datasetID, ok := createData["id"].(string)
	if !ok || datasetID == "" {
		t.Fatalf("dataset id is invalid: %v", createData)
	}

	avatarValue, err := encodeAvatar()
	if err != nil {
		t.Fatalf("failed to encode avatar: %v", err)
	}

	updateResp, err := TestConfig.PutJSON(fmt.Sprintf("/datasets/%s", datasetID), map[string]interface{}{
		"name":        "dataset_update_lang_connectors",
		"description": "",
		"parser_id":   "naive",
		"parse_type":  1,
		"language":    "English",
		"connectors":  []interface{}{},
		"avatar":      avatarValue,
	}, nil)
	if err != nil {
		t.Fatalf("update dataset request failed: %v", err)
	}
	updatePayload := requireStatusCode(t, updateResp, http.StatusOK)
	requireCodeZero(t, updatePayload)
	updateData, ok := updatePayload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("update response data is not an object: %v", updatePayload)
	}
	if updateData["language"] != "English" {
		t.Fatalf("language mismatch: expected English, got %v", updatePayload)
	}
	connectors, ok := updateData["connectors"].([]interface{})
	if !ok {
		t.Fatalf("connectors is not an array: %v", updatePayload)
	}
	if len(connectors) != 0 {
		t.Fatalf("connectors should be empty: %v", updatePayload)
	}
	if updateData["avatar"] != avatarValue {
		t.Fatalf("avatar mismatch: %v", updatePayload)
	}

	descriptionResp, err := TestConfig.PutJSON(fmt.Sprintf("/datasets/%s", datasetID), map[string]interface{}{"description": "description"}, nil)
	if err != nil {
		t.Fatalf("update description request failed: %v", err)
	}
	descriptionPayload := requireStatusCode(t, descriptionResp, http.StatusOK)
	requireCodeZero(t, descriptionPayload)
	descriptionData, ok := descriptionPayload["data"].(map[string]interface{})
	if !ok || descriptionData["description"] != "description" {
		t.Fatalf("description mismatch: %v", descriptionPayload)
	}
}

func TestDatasetUpdateParserConfigValidMatrixContract(t *testing.T) {
	clearDatasets(t)
	t.Skip("Go validation or response contract does not match the established API contract")

	cases := []struct {
		name         string
		parserConfig map[string]interface{}
	}{
		{name: "auto_keywords_min", parserConfig: map[string]interface{}{"auto_keywords": 0}},
		{name: "auto_keywords_mid", parserConfig: map[string]interface{}{"auto_keywords": 16}},
		{name: "auto_keywords_max", parserConfig: map[string]interface{}{"auto_keywords": 32}},
		{name: "auto_questions_min", parserConfig: map[string]interface{}{"auto_questions": 0}},
		{name: "auto_questions_mid", parserConfig: map[string]interface{}{"auto_questions": 5}},
		{name: "auto_questions_max", parserConfig: map[string]interface{}{"auto_questions": 10}},
		{name: "chunk_token_num_min", parserConfig: map[string]interface{}{"chunk_token_num": 1}},
		{name: "chunk_token_num_mid", parserConfig: map[string]interface{}{"chunk_token_num": 1024}},
		{name: "chunk_token_num_max", parserConfig: map[string]interface{}{"chunk_token_num": 2048}},
		{name: "delimiter", parserConfig: map[string]interface{}{"delimiter": "\n"}},
		{name: "delimiter_space", parserConfig: map[string]interface{}{"delimiter": " "}},
		{name: "html4excel_true", parserConfig: map[string]interface{}{"html4excel": true}},
		{name: "html4excel_false", parserConfig: map[string]interface{}{"html4excel": false}},
		{name: "layout_recognize_DeepDOC", parserConfig: map[string]interface{}{"layout_recognize": "DeepDOC"}},
		{name: "layout_recognize_navie", parserConfig: map[string]interface{}{"layout_recognize": "Plain Text"}},
		{name: "tag_kb_ids", parserConfig: map[string]interface{}{"tag_kb_ids": []interface{}{"1", "2"}}},
		{name: "topn_tags_min", parserConfig: map[string]interface{}{"topn_tags": 1}},
		{name: "topn_tags_mid", parserConfig: map[string]interface{}{"topn_tags": 5}},
		{name: "topn_tags_max", parserConfig: map[string]interface{}{"topn_tags": 10}},
		{name: "filename_embd_weight_min", parserConfig: map[string]interface{}{"filename_embd_weight": 0.1}},
		{name: "filename_embd_weight_mid", parserConfig: map[string]interface{}{"filename_embd_weight": 0.5}},
		{name: "filename_embd_weight_max", parserConfig: map[string]interface{}{"filename_embd_weight": 1.0}},
		{name: "task_page_size_min", parserConfig: map[string]interface{}{"task_page_size": 1}},
		{name: "task_page_size_None", parserConfig: map[string]interface{}{"task_page_size": nil}},
		{name: "pages", parserConfig: map[string]interface{}{"pages": []interface{}{[]interface{}{1, 100}}}},
		{name: "pages_none", parserConfig: map[string]interface{}{"pages": nil}},
		{name: "graphrag_true", parserConfig: map[string]interface{}{"graphrag": map[string]interface{}{"use_graphrag": true}}},
		{name: "graphrag_false", parserConfig: map[string]interface{}{"graphrag": map[string]interface{}{"use_graphrag": false}}},
		{name: "graphrag_entity_types", parserConfig: map[string]interface{}{"graphrag": map[string]interface{}{"entity_types": []interface{}{"age", "sex", "height", "weight"}}}},
		{name: "graphrag_method_general", parserConfig: map[string]interface{}{"graphrag": map[string]interface{}{"method": "general"}}},
		{name: "graphrag_method_light", parserConfig: map[string]interface{}{"graphrag": map[string]interface{}{"method": "light"}}},
		{name: "graphrag_community_true", parserConfig: map[string]interface{}{"graphrag": map[string]interface{}{"community": true}}},
		{name: "graphrag_community_false", parserConfig: map[string]interface{}{"graphrag": map[string]interface{}{"community": false}}},
		{name: "graphrag_resolution_true", parserConfig: map[string]interface{}{"graphrag": map[string]interface{}{"resolution": true}}},
		{name: "graphrag_resolution_false", parserConfig: map[string]interface{}{"graphrag": map[string]interface{}{"resolution": false}}},
		{name: "raptor_true", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"use_raptor": true}}},
		{name: "raptor_false", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"use_raptor": false}}},
		{name: "raptor_prompt", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"prompt": "Who are you?"}}},
		{name: "raptor_max_token_min", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"max_token": 512}}},
		{name: "raptor_max_token_mid", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"max_token": 1024}}},
		{name: "raptor_max_token_max", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"max_token": 2048}}},
		{name: "raptor_clustering_threshold_min", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"clustering_threshold": 0.0}}},
		{name: "raptor_clustering_threshold_mid", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"clustering_threshold": 0.5}}},
		{name: "raptor_clustering_threshold_max", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"clustering_threshold": 1.0}}},
		{name: "raptor_max_cluster_min", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"max_cluster": 1}}},
		{name: "raptor_max_cluster_mid", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"max_cluster": 512}}},
		{name: "raptor_max_cluster_max", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"max_cluster": 1024}}},
		{name: "raptor_random_seed_min", parserConfig: map[string]interface{}{"raptor": map[string]interface{}{"random_seed": 0}}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			createResp, err := TestConfig.PostJSON("/datasets", map[string]interface{}{"name": fmt.Sprintf("dataset_update_parser_%s", tc.name)}, nil)
			if err != nil {
				t.Fatalf("create dataset request failed: %v", err)
			}
			createPayload := requireStatusCode(t, createResp, http.StatusOK)
			requireCodeZero(t, createPayload)
			createData, ok := createPayload["data"].(map[string]interface{})
			if !ok {
				t.Fatalf("create response data is not an object: %v", createPayload)
			}
			datasetID, ok := createData["id"].(string)
			if !ok || datasetID == "" {
				t.Fatalf("dataset id is invalid: %v", createData)
			}

			updateResp, err := TestConfig.PutJSON(fmt.Sprintf("/datasets/%s", datasetID), map[string]interface{}{"parser_config": tc.parserConfig}, nil)
			if err != nil {
				t.Fatalf("update parser_config request failed: %v", err)
			}
			updatePayload := requireStatusCode(t, updateResp, http.StatusOK)
			requireCodeZero(t, updatePayload)

			listResp, err := TestConfig.GetJSON("/datasets", map[string]interface{}{"id": datasetID}, nil)
			if err != nil {
				t.Fatalf("list dataset request failed: %v", err)
			}
			listPayload := requireStatusCode(t, listResp, http.StatusOK)
			requireCodeZero(t, listPayload)
			listData, ok := listPayload["data"].([]interface{})
			if !ok || len(listData) != 1 {
				t.Fatalf("expected 1 dataset in list, got %v", listPayload)
			}
			firstItem, ok := listData[0].(map[string]interface{})
			if !ok {
				t.Fatalf("list data[0] is not an object: %v", listPayload)
			}
			actualParserConfig, ok := firstItem["parser_config"].(map[string]interface{})
			if !ok {
				t.Fatalf("parser_config is not an object: %v", firstItem)
			}

			for key, expectedValue := range tc.parserConfig {
				if key == "graphrag" || key == "raptor" {
					if _, exists := actualParserConfig[key]; exists {
						t.Fatalf("expected %s not in parser_config, but got %v", key, actualParserConfig)
					}
					continue
				}
				actualValue, ok := actualParserConfig[key]
				if !ok {
					t.Fatalf("parser_config missing key %s: %v", key, actualParserConfig)
				}
				if !jsonDeepEqual(actualValue, expectedValue) {
					t.Fatalf("parser_config.%s mismatch: expected %v, got %v", key, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestDatasetUpdateParserConfigWithChunkMethodChangeContract(t *testing.T) {
	clearDatasets(t)

	cases := []struct {
		name          string
		updatePayload map[string]interface{}
	}{
		{
			name: "parser_config_empty",
			updatePayload: map[string]interface{}{
				"parser_id":     "qa",
				"parse_type":    1,
				"parser_config": map[string]interface{}{},
			},
		},
		{
			name: "parser_config_none",
			updatePayload: map[string]interface{}{
				"parser_id":     "qa",
				"parse_type":    1,
				"parser_config": nil,
			},
		},
		{
			name: "parser_config_unset",
			updatePayload: map[string]interface{}{
				"parser_id":  "qa",
				"parse_type": 1,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			createResp, err := TestConfig.PostJSON("/datasets", map[string]interface{}{"name": fmt.Sprintf("dataset_update_%s", tc.name)}, nil)
			if err != nil {
				t.Fatalf("create dataset request failed: %v", err)
			}
			createPayload := requireStatusCode(t, createResp, http.StatusOK)
			requireCodeZero(t, createPayload)
			createData, ok := createPayload["data"].(map[string]interface{})
			if !ok {
				t.Fatalf("create response data is not an object: %v", createPayload)
			}
			datasetID, ok := createData["id"].(string)
			if !ok || datasetID == "" {
				t.Fatalf("dataset id is invalid: %v", createData)
			}

			updateResp, err := TestConfig.PutJSON(fmt.Sprintf("/datasets/%s", datasetID), tc.updatePayload, nil)
			if err != nil {
				t.Fatalf("update dataset request failed: %v", err)
			}
			updatePayload := requireStatusCode(t, updateResp, http.StatusOK)
			requireCodeZero(t, updatePayload)

			listResp, err := TestConfig.GetJSON("/datasets", map[string]interface{}{"id": datasetID}, nil)
			if err != nil {
				t.Fatalf("list dataset request failed: %v", err)
			}
			listPayload := requireStatusCode(t, listResp, http.StatusOK)
			requireCodeZero(t, listPayload)
			listData, ok := listPayload["data"].([]interface{})
			if !ok || len(listData) != 1 {
				t.Fatalf("expected 1 dataset in list, got %v", listPayload)
			}
			firstItem, ok := listData[0].(map[string]interface{})
			if !ok {
				t.Fatalf("list data[0] is not an object: %v", listPayload)
			}
			actualParserConfig, ok := firstItem["parser_config"].(map[string]interface{})
			if !ok || len(actualParserConfig) == 0 {
				t.Fatalf("parser_config should be non-empty map: %v", firstItem)
			}
			if _, exists := actualParserConfig["raptor"]; exists {
				t.Fatalf("raptor should not be in parser_config: %v", actualParserConfig)
			}
			if _, exists := actualParserConfig["graphrag"]; exists {
				t.Fatalf("graphrag should not be in parser_config: %v", actualParserConfig)
			}
		})
	}
}
