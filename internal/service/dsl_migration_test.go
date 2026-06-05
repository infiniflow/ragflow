package service

import (
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

func TestDeepCopyJSONLike_PreservesTypes(t *testing.T) {
	original := entity.JSONMap{
		"intVal":    512,
		"floatVal":  3.14,
		"stringVal": "test",
		"boolVal":   true,
		"nestedMap": map[string]interface{}{
			"subInt": 1024,
		},
		"sliceVal": []interface{}{
			1, 2.5, "three",
		},
	}

	copied := deepCopyJSONLike(original)

	// Modify original to ensure it's a deep copy
	original["intVal"] = 999
	original["stringVal"] = "modified"
	originalMap := original["nestedMap"].(map[string]interface{})
	originalMap["subInt"] = 2048
	originalSlice := original["sliceVal"].([]interface{})
	originalSlice[0] = 99

	copiedMap, ok := copied.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected copied value to be a map, got %T", copied)
	}

	// Verify types and values
	if val, ok := copiedMap["intVal"].(int); !ok || val != 512 {
		t.Errorf("Expected intVal to be int(512), got %T(%v)", copiedMap["intVal"], copiedMap["intVal"])
	}

	if val, ok := copiedMap["floatVal"].(float64); !ok || val != 3.14 {
		t.Errorf("Expected floatVal to be float64(3.14), got %T(%v)", copiedMap["floatVal"], copiedMap["floatVal"])
	}

	if val, ok := copiedMap["stringVal"].(string); !ok || val != "test" {
		t.Errorf("Expected stringVal to be string(\"test\"), got %T(%v)", copiedMap["stringVal"], copiedMap["stringVal"])
	}

	nestedCopiedMap, ok := copiedMap["nestedMap"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected nestedMap to be map[string]interface{}")
	}

	if val, ok := nestedCopiedMap["subInt"].(int); !ok || val != 1024 {
		t.Errorf("Expected nestedMap.subInt to be int(1024), got %T(%v)", nestedCopiedMap["subInt"], nestedCopiedMap["subInt"])
	}

	sliceCopied, ok := copiedMap["sliceVal"].([]interface{})
	if !ok {
		t.Fatalf("Expected sliceVal to be []interface{}")
	}

	if val, ok := sliceCopied[0].(int); !ok || val != 1 {
		t.Errorf("Expected sliceVal[0] to be int(1), got %T(%v)", sliceCopied[0], sliceCopied[0])
	}
}

func TestNormalizeChunkerDSL(t *testing.T) {
	tests := []struct {
		name     string
		input    entity.JSONMap
		expected entity.JSONMap
	}{
		{
			name:     "Nil input",
			input:    nil,
			expected: nil,
		},
		{
			name: "No components",
			input: entity.JSONMap{
				"other_field": 123,
			},
			expected: entity.JSONMap{
				"other_field": 123,
			},
		},
		{
			name: "Rename node types, names, and references",
			input: entity.JSONMap{
				"components": map[string]interface{}{
					"Splitter:123": map[string]interface{}{
						"obj": map[string]interface{}{
							"component_name": "Splitter",
						},
						"downstream": []interface{}{"HierarchicalMerger:456", "Other:789"},
						"upstream":   []interface{}{},
					},
					"HierarchicalMerger:456": map[string]interface{}{
						"obj": map[string]interface{}{
							"component_name": "HierarchicalMerger",
						},
						"downstream": []interface{}{},
						"upstream":   []interface{}{"Splitter:123"},
					},
				},
				"graph": map[string]interface{}{
					"nodes": []interface{}{
						map[string]interface{}{
							"id":   "Splitter:123",
							"type": "splitterNode",
							"data": map[string]interface{}{
								"label": "Splitter",
								"name":  "Splitter",
							},
						},
						map[string]interface{}{
							"id":   "HierarchicalMerger:456",
							"type": "otherNode",
							"data": map[string]interface{}{
								"label": "HierarchicalMerger",
								"name":  "HierarchicalMerger",
							},
						},
					},
					"edges": []interface{}{
						map[string]interface{}{
							"id":     "edge-Splitter:123-HierarchicalMerger:456",
							"source": "Splitter:123",
							"target": "HierarchicalMerger:456",
						},
					},
				},
			},
			expected: entity.JSONMap{
				"components": map[string]interface{}{
					"TokenChunker:123": map[string]interface{}{
						"obj": map[string]interface{}{
							"component_name": "TokenChunker",
						},
						"downstream": []interface{}{"TitleChunker:456", "Other:789"},
						"upstream":   []interface{}{},
					},
					"TitleChunker:456": map[string]interface{}{
						"obj": map[string]interface{}{
							"component_name": "TitleChunker",
						},
						"downstream": []interface{}{},
						"upstream":   []interface{}{"TokenChunker:123"},
					},
				},
				"graph": map[string]interface{}{
					"nodes": []interface{}{
						map[string]interface{}{
							"id":   "TokenChunker:123",
							"type": "chunkerNode",
							"data": map[string]interface{}{
								"label": "TokenChunker",
								"name":  "TokenChunker",
							},
						},
						map[string]interface{}{
							"id":   "TitleChunker:456",
							"type": "otherNode",
							"data": map[string]interface{}{
								"label": "TitleChunker",
								"name":  "TitleChunker",
							},
						},
					},
					"edges": []interface{}{
						map[string]interface{}{
							"id":     "edge-TokenChunker:123-TitleChunker:456",
							"source": "TokenChunker:123",
							"target": "TitleChunker:456",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeChunkerDSL(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("NormalizeChunkerDSL() = %v, want %v", result, tt.expected)
			}
		})
	}
}
