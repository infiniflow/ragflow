package cli

import (
	"reflect"
	"testing"
)

func TestParseAddModelWithDimensions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *Command
		wantErr  bool
	}{
		{
			name:  "Add model with detailed embedding dimensions",
			input: "add model 'x1 x2 x3 x4 x5' to provider 'vllm' instance 'test' with tokens 1024 chat think vision, token 2048 chat, token 1024 think vision, token 0 embedding 2048 64 1024 2048, token 0 embedding 2048;",
			expected: &Command{
				Type: "add_custom_model",
				Params: map[string]interface{}{
					"provider_name": "vllm",
					"instance_name": "test",
					"models": []map[string]interface{}{
						{
							"model_name":  "x1",
							"model_types": []string{"chat", "vision"},
							"max_tokens":  1024,
							"thinking":    true,
						},
						{
							"model_name":  "x2",
							"model_types": []string{"chat"},
							"max_tokens":  2048,
						},
						{
							"model_name":  "x3",
							"model_types": []string{"vision"},
							"max_tokens":  1024,
							"thinking":    true,
						},
						{
							"model_name":    "x4",
							"model_types":   []string{"embedding"},
							"max_tokens":    0,
							"max_dimension": 2048,
							"dimensions":    []int{64, 1024, 2048},
						},
						{
							"model_name":    "x5",
							"model_types":   []string{"embedding"},
							"max_tokens":    0,
							"max_dimension": 2048,
							"dimensions":    []int{},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(tt.input)
			cmd, err := p.Parse(APIMode)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if cmd.Type != tt.expected.Type {
				t.Errorf("Command Type = %v, expected = %v", cmd.Type, tt.expected.Type)
			}

			// Validate provider name
			gotProvider, _ := cmd.Params["provider_name"].(string)
			expectedProvider, _ := tt.expected.Params["provider_name"].(string)
			if gotProvider != expectedProvider {
				t.Errorf("provider_name = %v, expected = %v", gotProvider, expectedProvider)
			}

			// Validate instance name
			gotInstance, _ := cmd.Params["instance_name"].(string)
			expectedInstance, _ := tt.expected.Params["instance_name"].(string)
			if gotInstance != expectedInstance {
				t.Errorf("instance_name = %v, expected = %v", gotInstance, expectedInstance)
			}

			// Validate models
			gotModels, ok1 := cmd.Params["models"].([]map[string]interface{})
			if !ok1 {
				// Try another type just in case type conversion differs
				gotModelsAny, okAny := cmd.Params["models"].([]map[string]any)
				if okAny {
					gotModels = make([]map[string]interface{}, len(gotModelsAny))
					for idx, val := range gotModelsAny {
						gotModels[idx] = val
					}
					ok1 = true
				}
			}
			expectedModels, _ := tt.expected.Params["models"].([]map[string]interface{})

			if !ok1 {
				t.Fatalf("models param not found or has incorrect type: %T", cmd.Params["models"])
			}

			if len(gotModels) != len(expectedModels) {
				t.Fatalf("len(models) = %d, expected = %d", len(gotModels), len(expectedModels))
			}

			for idx := range gotModels {
				gotModel := gotModels[idx]
				expectedModel := expectedModels[idx]

				if gotModel["model_name"] != expectedModel["model_name"] {
					t.Errorf("model[%d].model_name = %v, expected = %v", idx, gotModel["model_name"], expectedModel["model_name"])
				}

				if !reflect.DeepEqual(gotModel["model_types"], expectedModel["model_types"]) {
					t.Errorf("model[%d].model_types = %v, expected = %v", idx, gotModel["model_types"], expectedModel["model_types"])
				}

				if gotModel["max_tokens"] != expectedModel["max_tokens"] {
					t.Errorf("model[%d].max_tokens = %v, expected = %v", idx, gotModel["max_tokens"], expectedModel["max_tokens"])
				}

				if gotModel["thinking"] != expectedModel["thinking"] {
					t.Errorf("model[%d].thinking = %v, expected = %v", idx, gotModel["thinking"], expectedModel["thinking"])
				}

				if gotModel["max_dimension"] != expectedModel["max_dimension"] {
					t.Errorf("model[%d].max_dimension = %v, expected = %v", idx, gotModel["max_dimension"], expectedModel["max_dimension"])
				}

				if expectedModel["dimensions"] != nil {
					if !reflect.DeepEqual(gotModel["dimensions"], expectedModel["dimensions"]) {
						t.Errorf("model[%d].dimensions = %v, expected = %v", idx, gotModel["dimensions"], expectedModel["dimensions"])
					}
				}
			}
		})
	}
}
