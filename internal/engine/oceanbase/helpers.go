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

package oceanbase

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

func (e *Engine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{}, len(chunks))
	if len(fields) == 0 {
		return result
	}
	for _, chunk := range chunks {
		id := stringValue(chunk["id"])
		if id == "" {
			id = stringValue(chunk["skill_id"])
		}
		if id == "" {
			continue
		}
		selected := make(map[string]interface{}, len(fields))
		for _, field := range fields {
			if value, ok := chunk[field]; ok {
				selected[field] = value
			} else {
				selected[field] = nil
			}
		}
		result[id] = selected
	}
	return result
}

func (e *Engine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	counts := make(map[string]int)
	values := make([]string, 0)
	addValue := func(value string) {
		if counts[value] == 0 {
			values = append(values, value)
		}
		counts[value]++
	}
	for _, chunk := range chunks {
		value := chunk[fieldName]
		if items, ok := interfaceSlice(value); ok {
			for _, item := range items {
				text, ok := item.(string)
				if ok && strings.TrimSpace(text) != "" {
					addValue(text)
				}
			}
			continue
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		addValue(text)
	}
	result := make([]map[string]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]interface{}{"key": value, "count": counts[value]})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i]["count"].(int) > result[j]["count"].(int)
	})
	return result
}

func (e *Engine) GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
	result := make(map[string]string)
	marker := newHighlightMarker(keywords)
	for _, chunk := range chunks {
		id := stringValue(chunk["id"])
		if id == "" {
			id = stringValue(chunk["skill_id"])
		}
		text := stringValue(chunk[fieldName])
		if id == "" || text == "" {
			continue
		}
		tokenizedText := ""
		if fieldName == "content_with_weight" {
			tokenizedText = stringValue(chunk["content_ltks"])
		}
		if highlighted := marker.markText(text, tokenizedText); highlighted != "" {
			result[id] = highlighted
		}
	}
	return result
}

func (e *Engine) GetChunkIDs(chunks []map[string]interface{}) []string {
	result := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if id := stringValue(chunk["id"]); id != "" {
			result = append(result, id)
		} else if id := stringValue(chunk["skill_id"]); id != "" {
			result = append(result, id)
		}
	}
	return result
}

// KNNScores computes clean cosine scores from vectors selected with the first
// query. Retrieval includes q_<dim>_vec in OceanBase-family source fields.
func (e *Engine) KNNScores(ctx context.Context, chunks []map[string]interface{}, queryVector []float64, topK int) (map[string]interface{}, error) {
	if len(chunks) == 0 || len(queryVector) == 0 {
		return nil, nil
	}
	vectorField := fmt.Sprintf("q_%d_vec", len(queryVector))
	hits := make([]interface{}, 0, len(chunks))
	for _, chunk := range chunks {
		vector, ok := floatSlice(chunk[vectorField])
		if !ok {
			continue
		}
		hits = append(hits, map[string]interface{}{"_id": stringValue(chunk["id"]), "_score": cosineSimilarity(queryVector, vector)})
	}
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].(map[string]interface{})["_score"].(float64) > hits[j].(map[string]interface{})["_score"].(float64)
	})
	if topK > 0 && len(hits) > topK {
		hits = hits[:topK]
	}
	return map[string]interface{}{"hits": map[string]interface{}{"hits": hits}}, nil
}

func (e *Engine) GetScores(knnResult map[string]interface{}) map[string]float64 {
	result := make(map[string]float64)
	if knnResult == nil {
		return result
	}
	hitsObject, _ := knnResult["hits"].(map[string]interface{})
	hits, _ := hitsObject["hits"].([]interface{})
	for _, raw := range hits {
		hit, _ := raw.(map[string]interface{})
		id := stringValue(hit["_id"])
		if score, ok := numberToFloat(hit["_score"]); ok && id != "" {
			result[id] = score
		}
	}
	return result
}

func cosineSimilarity(left, right []float64) float64 {
	if len(left) != len(right) || len(left) == 0 {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func numberToFloat(value interface{}) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
