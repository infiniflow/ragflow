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

package service

import (
	"encoding/json"
	"fmt"
	"math"
)

const similarityWeightTolerance = 1e-9

// ResolveVectorSimilarityWeight validates the public keyword/vector weights and
// returns the vector weight used by retrieval. Keyword weight takes precedence.
func ResolveVectorSimilarityWeight(keywordsSimilarityWeight, vectorSimilarityWeight *float64) (*float64, error) {
	if keywordsSimilarityWeight != nil && (*keywordsSimilarityWeight < 0 || *keywordsSimilarityWeight > 1) {
		return nil, fmt.Errorf("keywords_similarity_weight must be between 0 and 1")
	}
	if vectorSimilarityWeight != nil && (*vectorSimilarityWeight < 0 || *vectorSimilarityWeight > 1) {
		return nil, fmt.Errorf("vector_similarity_weight must be between 0 and 1")
	}
	if keywordsSimilarityWeight != nil && vectorSimilarityWeight != nil && math.Abs(*keywordsSimilarityWeight+*vectorSimilarityWeight-1) > similarityWeightTolerance {
		return nil, fmt.Errorf("keywords_similarity_weight and vector_similarity_weight must sum to 1")
	}
	if keywordsSimilarityWeight != nil {
		resolved := 1 - *keywordsSimilarityWeight
		return &resolved, nil
	}
	return vectorSimilarityWeight, nil
}

// NormalizeSimilarityWeights validates similarity weights in a REST payload and
// synchronizes both field names. Payloads that omit both fields are unchanged.
func NormalizeSimilarityWeights(values map[string]interface{}) error {
	keywordsSimilarityWeight, err := similarityWeightFromMap(values, "keywords_similarity_weight")
	if err != nil {
		return err
	}
	vectorSimilarityWeight, err := similarityWeightFromMap(values, "vector_similarity_weight")
	if err != nil {
		return err
	}

	resolvedVectorWeight, err := ResolveVectorSimilarityWeight(keywordsSimilarityWeight, vectorSimilarityWeight)
	if err != nil {
		return err
	}
	if resolvedVectorWeight == nil {
		return nil
	}

	values["vector_similarity_weight"] = *resolvedVectorWeight
	values["keywords_similarity_weight"] = 1 - *resolvedVectorWeight
	return nil
}

func similarityWeightFromMap(values map[string]interface{}, key string) (*float64, error) {
	value, ok := values[key]
	if !ok || value == nil {
		return nil, nil
	}

	var weight float64
	switch typed := value.(type) {
	case float64:
		weight = typed
	case float32:
		weight = float64(typed)
	case int:
		weight = float64(typed)
	case int32:
		weight = float64(typed)
	case int64:
		weight = float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return nil, fmt.Errorf("%s must be a number", key)
		}
		weight = parsed
	default:
		return nil, fmt.Errorf("%s must be a number", key)
	}
	return &weight, nil
}
