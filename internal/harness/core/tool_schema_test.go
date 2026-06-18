package core

import (
	"context"
	"testing"
)

// === Test struct for schema generation ===

type weatherArgs struct {
	City    string  `json:"city" description:"The city name" required:"true"`
	Country string  `json:"country,omitempty" description:"Optional country name"`
	Temp    float64 `json:"temp" description:"Temperature" required:"true"`
	Units   string  `json:"units" enum:"metric,imperial" description:"Temperature units"`
}

type emptyArgs struct{}

type nestedArgs struct {
	Query string      `json:"query" description:"Search query"`
	Page  int         `json:"page" description:"Page number"`
	Tags  []string    `json:"tags" description:"Filter tags"`
}

// ======================== GenerateToolInfo ========================

func TestGenerateToolInfo_Basic(t *testing.T) {
	fn := func(ctx context.Context, args *weatherArgs) (string, error) {
		return "sunny", nil
	}

	info, err := GenerateToolInfo[weatherArgs]("get_weather", "Get current weather", fn)
	if err != nil {
		t.Fatalf("GenerateToolInfo: %v", err)
	}
	if info.Name != "get_weather" {
		t.Errorf("expected 'get_weather', got %s", info.Name)
	}
	if info.Description != "Get current weather" {
		t.Errorf("expected 'Get current weather', got %s", info.Description)
	}
	if info.InputSchema == nil {
		t.Fatal("expected non-nil InputSchema")
	}

	props, ok := info.InputSchema.(map[string]interface{})["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}

	// Verify each field exists with correct type
	cityProp, ok := props["city"].(map[string]interface{})
	if !ok {
		t.Fatal("expected city property schema")
	}
	if cityProp["type"] != "string" {
		t.Errorf("expected city type 'string', got %v", cityProp["type"])
	}
	if cityProp["description"] != "The city name" {
		t.Errorf("expected city description 'The city name', got %v", cityProp["description"])
	}

	tempProp, ok := props["temp"].(map[string]interface{})
	if !ok {
		t.Fatal("expected temp property schema")
	}
	if tempProp["type"] != "number" {
		t.Errorf("expected temp type 'number', got %v", tempProp["type"])
	}

	unitsProp, ok := props["units"].(map[string]interface{})
	if !ok {
		t.Fatal("expected units property schema")
	}
	if unitsProp["type"] != "string" {
		t.Errorf("expected units type 'string', got %v", unitsProp["type"])
	}
}

func TestGenerateToolInfo_EmptyStruct(t *testing.T) {
	fn := func(ctx context.Context, args *emptyArgs) (string, error) {
		return "no args", nil
	}

	info, err := GenerateToolInfo[emptyArgs]("noop", "Does nothing", fn)
	if err != nil {
		t.Fatalf("GenerateToolInfo: %v", err)
	}
	if info.Name != "noop" {
		t.Errorf("expected 'noop', got %s", info.Name)
	}
	props := info.InputSchema.(map[string]interface{})["properties"].(map[string]interface{})
	if len(props) != 0 {
		t.Errorf("expected 0 properties, got %d", len(props))
	}
}

func TestGenerateToolInfo_NestedTypes(t *testing.T) {
	fn := func(ctx context.Context, args *nestedArgs) (string, error) {
		return "nested", nil
	}

	info, err := GenerateToolInfo[nestedArgs]("search", "Search", fn)
	if err != nil {
		t.Fatalf("GenerateToolInfo: %v", err)
	}

	props := info.InputSchema.(map[string]interface{})["properties"].(map[string]interface{})
	pageProp := props["page"].(map[string]interface{})
	if pageProp["type"] != "integer" {
		t.Errorf("expected page type 'integer', got %v", pageProp["type"])
	}

	tagsProp := props["tags"].(map[string]interface{})
	if tagsProp["type"] != "array" {
		t.Errorf("expected tags type 'array', got %v", tagsProp["type"])
	}
}

func TestGenerateToolInfo_PointerStruct(t *testing.T) {
	fn := func(ctx context.Context, args *weatherArgs) (string, error) { return "ok", nil }

	info, err := GenerateToolInfo[*weatherArgs]("ptr_test", "test", fn)
	if err != nil {
		t.Fatalf("GenerateToolInfo pointer: %v", err)
	}
	if info.Name != "ptr_test" {
		t.Errorf("expected 'ptr_test', got %s", info.Name)
	}
}

// ======================== ReflectTool ========================

func TestReflectTool_Basic(t *testing.T) {
	tool, err := ReflectTool("greet", "Greet someone",
		func(ctx context.Context, args *weatherArgs) (string, error) {
			return "Hello " + args.City, nil
		})
	if err != nil {
		t.Fatalf("ReflectTool: %v", err)
	}
	if tool.Name() != "greet" {
		t.Errorf("expected 'greet', got %s", tool.Name())
	}
	if tool.Description() != "Greet someone" {
		t.Errorf("expected 'Greet someone', got %s", tool.Description())
	}

	result, err := tool.Invoke(context.Background(), `{"city":"London"}`)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result != "Hello London" {
		t.Errorf("expected 'Hello London', got %s", result)
	}
}

func TestReflectTool_Stream(t *testing.T) {
	tool, err := ReflectTool("echo", "Echo",
		func(ctx context.Context, args *weatherArgs) (string, error) {
			return "echo: " + args.City, nil
		})
	if err != nil {
		t.Fatalf("ReflectTool: %v", err)
	}

	stream, err := tool.Stream(context.Background(), `{"city":"test"}`)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if stream == nil {
		t.Fatal("expected non-nil stream")
	}
}

func TestReflectTool_ToolInfo(t *testing.T) {
	tool, err := ReflectTool("info_test", "Info test",
		func(ctx context.Context, args *weatherArgs) (string, error) {
			return "info", nil
		})
	if err != nil {
		t.Fatalf("ReflectTool: %v", err)
	}

	info := tool.ToolInfo()
	if info.Name != "info_test" {
		t.Errorf("expected 'info_test', got %s", info.Name)
	}
}

func TestReflectTool_InvalidJSON(t *testing.T) {
	tool, err := ReflectTool("bad_json", "Bad JSON",
		func(ctx context.Context, args *weatherArgs) (string, error) {
			return "ok", nil
		})
	if err != nil {
		t.Fatalf("ReflectTool: %v", err)
	}

	_, err = tool.Invoke(context.Background(), `not valid json`)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMustReflectTool(t *testing.T) {
	tool := MustReflectTool("must", "Must tool",
		func(ctx context.Context, args *weatherArgs) (string, error) {
			return "must ok", nil
		})
	if tool.Name() != "must" {
		t.Errorf("expected 'must', got %s", tool.Name())
	}
}

func TestReflectTool_RegistryIntegration(t *testing.T) {
	r := NewToolRegistry()
	tool := MustReflectTool("registry_test", "Registry test",
		func(ctx context.Context, args *weatherArgs) (string, error) {
			return "registry ok", nil
		})
	r.Register(tool, WithCategory("test"))

	found, ok := r.Lookup("registry_test")
	if !ok {
		t.Fatal("expected to find tool in registry")
	}
	if found.Name() != "registry_test" {
		t.Errorf("expected 'registry_test', got %s", found.Name())
	}
}
