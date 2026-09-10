////go:build integration

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
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{B: 255, A: 255}}, image.Point{}, draw.Src)
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
