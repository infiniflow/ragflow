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
	"encoding/json"
	"fmt"
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
	if err := json.Unmarshal(body, &payload); err != nil {
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

func TestDatasetCRUDCycle(t *testing.T) {
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
