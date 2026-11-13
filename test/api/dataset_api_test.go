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
	"image"
	"image/color"
	"image/png"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// createRESTDataset registers ownership before checking the persisted resource.
// Explicit names are useful for duplicate and boundary tests; all callers own
// only the returned ID, even when the server deduplicates the requested name.
func createRESTDataset(t *testing.T, fields apiObject) apiObject {
	t.Helper()
	payload := apiObject{"name": uniqueName("go_dataset"), "parse_type": 1, "parser_id": "general"}
	for key, value := range fields {
		payload[key] = value
	}
	result := restCall(t, TestConfig, http.MethodPost, "/datasets", payload, nil)
	if data, ok := result.JSON["data"].(apiObject); ok {
		if id, ok := data["id"].(string); ok && id != "" {
			cleanupResource(t, TestConfig, "datasets", id)
		}
	}
	data := object(t, restOK(t, result)["data"])
	id, ok := data["id"].(string)
	if !ok || id == "" {
		t.Fatalf("created dataset has invalid id: %v", data)
	}
	persisted := getRESTDataset(t, id)
	for _, key := range []string{"id", "name", "permission", "parser_id", "embedding_model", "parser_config"} {
		if !reflect.DeepEqual(data[key], persisted[key]) {
			t.Fatalf("created %s differs from persisted value: response=%v stored=%v", key, data[key], persisted[key])
		}
	}
	return persisted
}

func getRESTDataset(t *testing.T, id string) apiObject {
	t.Helper()
	return object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/datasets/"+id, nil, nil))["data"])
}

func requireDatasetFields(t *testing.T, id string, expected apiObject) apiObject {
	t.Helper()
	actual := getRESTDataset(t, id)
	for key, want := range expected {
		if !reflect.DeepEqual(actual[key], want) {
			t.Errorf("dataset %s field %s=%#v want=%#v", id, key, actual[key], want)
		}
	}
	return actual
}

func requireDatasetAbsent(t *testing.T, id string) {
	t.Helper()
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets/"+id, nil, nil), 200, 404)
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets", nil, apiObject{"id": id}), 200, 404)
}

func TestDatasetCRUDCycle(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	id := dataset["id"].(string)
	name := uniqueName("renamed")
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"name": name}, nil))
	requireDatasetFields(t, id, apiObject{"name": name})
	listed := restOK(t, restCall(t, TestConfig, http.MethodGet, "/datasets", nil, apiObject{"id": id}))
	items := array(t, listed["data"])
	if len(items) != 1 || object(t, items[0])["id"] != id || listed["total_datasets"] != float64(1) {
		t.Fatalf("id-filtered list=%v", listed)
	}
	restOK(t, restCall(t, TestConfig, http.MethodDelete, "/datasets", apiObject{"ids": []string{id}}, nil))
	requireDatasetAbsent(t, id)
}

func TestDatasetUpdateNameAndCaseInsensitiveContract(t *testing.T) {
	first := createRESTDataset(t, nil)
	second := createRESTDataset(t, nil)
	id := first["id"].(string)
	name := uniqueName("renamed")
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"name": name}, nil))
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"name": strings.ToUpper(name)}, nil))
	requireDatasetFields(t, id, apiObject{"name": strings.ToUpper(name)})
	restCode(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"name": strings.ToUpper(second["name"].(string))}, nil), 200, 102)
	requireDatasetFields(t, id, apiObject{"name": strings.ToUpper(name)})
}

func encodeAvatar() (string, error) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func TestDatasetUpdateLanguageConnectorsAvatarAndDescriptionContract(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	id := dataset["id"].(string)
	avatar, err := encodeAvatar()
	if err != nil {
		t.Fatal(err)
	}
	fields := apiObject{"language": "English", "connectors": []any{}, "avatar": avatar, "description": "description"}
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, fields, nil))
	requireDatasetFields(t, id, fields)
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"description": ""}, nil))
	requireDatasetFields(t, id, apiObject{"description": "", "language": "English", "avatar": avatar, "connectors": []any{}})
}

func requirePipelineParserConfig(t *testing.T, value any, chunkerPrefix string) apiObject {
	t.Helper()
	config := object(t, value)
	if _, ok := config["File"]; !ok {
		t.Fatalf("parser config missing File node: %v", config)
	}
	parser, chunker := false, false
	for id, params := range config {
		object(t, params)
		parser = parser || strings.HasPrefix(id, "Parser:")
		chunker = chunker || strings.HasPrefix(id, chunkerPrefix)
	}
	if !parser || !chunker {
		t.Fatalf("parser config missing parser/chunker nodes: %v", config)
	}
	for _, key := range []string{"chunk_token_num", "raptor", "graphrag"} {
		if _, ok := config[key]; ok {
			t.Fatalf("unexpected flat setting %s: %v", key, config)
		}
	}
	return config
}

func TestDatasetUpdateParserConfigWithChunkMethodChangeContract(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provided bool
		config   any
	}{{"empty", true, apiObject{}}, {"null", true, nil}, {"unset", false, nil}} {
		t.Run(tc.name, func(t *testing.T) {
			dataset := createRESTDataset(t, nil)
			id := dataset["id"].(string)
			fields := apiObject{"parse_type": 1, "parser_id": "qa"}
			if tc.provided {
				fields["parser_config"] = tc.config
			}
			restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, fields, nil))
			persisted := requireDatasetFields(t, id, apiObject{"parser_id": "qa"})
			config := object(t, persisted["parser_config"])
			if len(config) == 0 {
				t.Fatal("parser config is empty after method change")
			}
			for _, key := range []string{"raptor", "graphrag"} {
				if _, ok := config[key]; ok {
					t.Fatalf("unexpected flat key %s", key)
				}
			}
		})
	}
}

func TestDatasetComponentScopedParserConfigRoundtrip(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	id := dataset["id"].(string)
	config := requirePipelineParserConfig(t, dataset["parser_config"], "GeneralChunker:")
	chunkerID := ""
	for key := range config {
		if strings.HasPrefix(key, "GeneralChunker:") {
			chunkerID = key
			break
		}
	}
	for _, delimiter := range []string{"|", ";", "\n"} {
		t.Run(strings.ReplaceAll(delimiter, "\n", "newline"), func(t *testing.T) {
			fields := apiObject{"parser_config": apiObject{chunkerID: apiObject{"parent_child": apiObject{"use_parent_child": true, "children_delimiter": delimiter}}}}
			restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, fields, nil))
			updated := object(t, getRESTDataset(t, id)["parser_config"])
			chunker := object(t, updated[chunkerID])
			if object(t, chunker["parent_child"])["use_parent_child"] != true || !reflect.DeepEqual(chunker["children_delimiters"], []any{delimiter}) {
				t.Fatalf("component settings did not reach chunker: %v", chunker)
			}
			for key, params := range config {
				if key != chunkerID && !reflect.DeepEqual(params, updated[key]) {
					t.Errorf("unrelated parser node %s changed", key)
				}
			}
		})
	}
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/datasets/"+id, apiObject{"parser_config": apiObject{"File": apiObject{"layout_recognize": "DeepDOC"}}}, nil))
	requirePipelineParserConfig(t, getRESTDataset(t, id)["parser_config"], "GeneralChunker:")
}
