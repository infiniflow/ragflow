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

package entity

const ingestionTaskRerunSchemaKey = "rerun"

// IngestionTaskRerunInfo carries the edited DSL a dataflow rerun should
// execute instead of the canvas DSL. It is stored on IngestionTask.Schema
// under the "rerun" key for traceability.
type IngestionTaskRerunInfo struct {
	DSL         JSONMap `json:"dsl"`
	LogID       string  `json:"log_id"`
	ComponentID string  `json:"component_id"`
}

// NewIngestionTaskRerunSchema builds the Schema payload for a rerun task.
func NewIngestionTaskRerunSchema(dsl JSONMap, logID, componentID string) JSONMap {
	return JSONMap{
		ingestionTaskRerunSchemaKey: map[string]interface{}{
			"dsl":          dsl,
			"log_id":       logID,
			"component_id": componentID,
		},
	}
}

// RerunInfo returns rerun metadata when the task was enqueued from a dataflow
// rerun request.
func (t *IngestionTask) RerunInfo() (IngestionTaskRerunInfo, bool) {
	if t == nil || t.Schema == nil {
		return IngestionTaskRerunInfo{}, false
	}
	raw, ok := t.Schema[ingestionTaskRerunSchemaKey]
	if !ok || raw == nil {
		return IngestionTaskRerunInfo{}, false
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return IngestionTaskRerunInfo{}, false
	}
	info := IngestionTaskRerunInfo{
		LogID:       stringField(m, "log_id"),
		ComponentID: stringField(m, "component_id"),
	}
	if dsl := jsonMapField(m, "dsl"); dsl != nil {
		info.DSL = dsl
		return info, true
	}
	return info, info.LogID != "" || info.ComponentID != ""
}

func jsonMapField(m map[string]interface{}, key string) JSONMap {
	switch v := m[key].(type) {
	case JSONMap:
		return v
	case map[string]interface{}:
		return JSONMap(v)
	default:
		return nil
	}
}

func stringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
