package cli

import (
	"reflect"
	"testing"
)

func TestParseChatMessageUsesCurrentModel(t *testing.T) {
	p := NewParser("chat message 'hi';")
	cmd, err := p.Parse(APIMode)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cmd.Type != "api_chat_to_model" {
		t.Fatalf("Command Type = %v, expected api_chat_to_model", cmd.Type)
	}
	if _, ok := cmd.Params["composite_model_name"]; ok {
		t.Fatal("composite_model_name should not be set")
	}
	if _, ok := cmd.Params["model_id"]; ok {
		t.Fatal("model_id should not be set")
	}

	gotMessages, ok := cmd.Params["messages"].([]string)
	if !ok {
		t.Fatalf("messages param has type %T, expected []string", cmd.Params["messages"])
	}
	if !reflect.DeepEqual(gotMessages, []string{"hi"}) {
		t.Fatalf("messages = %v, expected [hi]", gotMessages)
	}
}

func TestParseAddModelWithDimensions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *Command
		wantErr  bool
	}{
		{
			name:  "Add model with detailed embedding dimensions",
			input: "add model 'x1 x2 x3 x4 x5' to provider 'vllm' instance 'test' tokens 1024 chat think vision, token 2048 chat, token 1024 think vision, token 0 embedding 2048 64 1024 2048, token 0 embedding 2048;",
			expected: &Command{
				Type: "api_add_custom_model",
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

func TestParseListSyncLogs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *Command
		wantErr  bool
	}{
		{
			name:  "LIST SYNC_LOGS",
			input: "LIST SYNC_LOGS;",
			expected: &Command{
				Type:   "api_list_sync_logs",
				Params: map[string]interface{}{},
			},
		},
		{
			name:  "LIST SYNC_LOGS FROM dataset_id",
			input: "LIST SYNC_LOGS FROM 'kb-1';",
			expected: &Command{
				Type: "api_list_sync_logs",
				Params: map[string]interface{}{
					"dataset_id": "kb-1",
				},
			},
		},
		{
			name:  "LIST DATASET name SYNC_LOGS",
			input: "LIST DATASET 'my dataset' SYNC_LOGS;",
			expected: &Command{
				Type: "api_list_sync_logs",
				Params: map[string]interface{}{
					"dataset_name": "my dataset",
				},
			},
		},
		{
			name:  "LIST SYNC_LOGS WITH page and page_size",
			input: "LIST SYNC_LOGS WITH PAGE 2 PAGE_SIZE 50;",
			expected: &Command{
				Type: "api_list_sync_logs",
				Params: map[string]interface{}{
					"page":      2,
					"page_size": 50,
				},
			},
		},
		{
			name:  "LIST SYNC_LOGS FROM dataset_id WITH page",
			input: "LIST SYNC_LOGS FROM 'kb-1' WITH PAGE 3;",
			expected: &Command{
				Type: "api_list_sync_logs",
				Params: map[string]interface{}{
					"dataset_id": "kb-1",
					"page":       3,
				},
			},
		},
		{
			name:  "LIST DATASET name SYNC_LOGS WITH page_size",
			input: "LIST DATASET 'my dataset' SYNC_LOGS WITH PAGE_SIZE 20;",
			expected: &Command{
				Type: "api_list_sync_logs",
				Params: map[string]interface{}{
					"dataset_name": "my dataset",
					"page_size":    20,
				},
			},
		},
		{
			name:    "unknown WITH option",
			input:   "LIST SYNC_LOGS WITH FOO 1;",
			wantErr: true,
		},
		{
			name:    "non-integer WITH option",
			input:   "LIST SYNC_LOGS WITH PAGE 'x';",
			wantErr: true,
		},
		{
			name:    "comma-separated WITH options",
			input:   "LIST SYNC_LOGS WITH PAGE 2, PAGE_SIZE 50;",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(tt.input)
			cmd, err := p.Parse(APIMode)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if cmd.Type != tt.expected.Type {
				t.Errorf("Command Type = %v, expected = %v", cmd.Type, tt.expected.Type)
			}

			gotDatasetID, _ := cmd.Params["dataset_id"].(string)
			expectedDatasetID, _ := tt.expected.Params["dataset_id"].(string)
			if gotDatasetID != expectedDatasetID {
				t.Errorf("dataset_id = %v, expected = %v", gotDatasetID, expectedDatasetID)
			}

			gotDatasetName, _ := cmd.Params["dataset_name"].(string)
			expectedDatasetName, _ := tt.expected.Params["dataset_name"].(string)
			if gotDatasetName != expectedDatasetName {
				t.Errorf("dataset_name = %v, expected = %v", gotDatasetName, expectedDatasetName)
			}

			gotPage, _ := cmd.Params["page"].(int)
			expectedPage, _ := tt.expected.Params["page"].(int)
			if gotPage != expectedPage {
				t.Errorf("page = %v, expected = %v", gotPage, expectedPage)
			}

			gotPageSize, _ := cmd.Params["page_size"].(int)
			expectedPageSize, _ := tt.expected.Params["page_size"].(int)
			if gotPageSize != expectedPageSize {
				t.Errorf("page_size = %v, expected = %v", gotPageSize, expectedPageSize)
			}
		})
	}
}
