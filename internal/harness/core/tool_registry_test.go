package core

import (
	"context"
	"testing"

	"ragflow/internal/harness/core/schema"
)

func TestToolRegistry_RegisterAndLookup(t *testing.T) {
	r := NewToolRegistry()
	tool := newTestTool("get_weather", "Get weather")
	r.Register(tool)

	found, ok := r.Lookup("get_weather")
	if !ok {
		t.Fatal("expected to find 'get_weather'")
	}
	if found.Name() != "get_weather" {
		t.Errorf("expected 'get_weather', got %s", found.Name())
	}
}

func TestToolRegistry_LookupNotFound(t *testing.T) {
	r := NewToolRegistry()
	_, ok := r.Lookup("nonexistent")
	if ok {
		t.Error("expected false for nonexistent tool")
	}
}

func TestToolRegistry_WithAlias(t *testing.T) {
	r := NewToolRegistry()
	tool := newTestTool("web_search_v2", "Search web")
	r.Register(tool, WithAlias("search"))

	// Lookup by alias
	found, ok := r.Lookup("search")
	if !ok {
		t.Fatal("expected to find via alias 'search'")
	}
	if found.Name() != "web_search_v2" {
		t.Errorf("expected 'web_search_v2', got %s", found.Name())
	}

	// Lookup by canonical name still works
	found, ok = r.Lookup("web_search_v2")
	if !ok {
		t.Fatal("expected to find via canonical name")
	}
}

func TestToolRegistry_WithCategory(t *testing.T) {
	r := NewToolRegistry()
	r.Register(newTestTool("cat_tool1", ""), WithCategory("file", "read"))
	r.Register(newTestTool("cat_tool2", ""), WithCategory("file"))
	r.Register(newTestTool("other_tool", ""), WithCategory("network"))

	fileTools := r.LookupByCategory("file")
	if len(fileTools) != 2 {
		t.Errorf("expected 2 file tools, got %d", len(fileTools))
	}

	readTools := r.LookupByCategory("read")
	if len(readTools) != 1 {
		t.Errorf("expected 1 read tool, got %d", len(readTools))
	}

	networkTools := r.LookupByCategory("network")
	if len(networkTools) != 1 {
		t.Errorf("expected 1 network tool, got %d", len(networkTools))
	}

	emptyTools := r.LookupByCategory("nonexistent")
	if len(emptyTools) != 0 {
		t.Errorf("expected 0 tools for nonexistent category, got %d", len(emptyTools))
	}
}

func TestToolRegistry_AllTools(t *testing.T) {
	r := NewToolRegistry()
	r.Register(newTestTool("a", ""))
	r.Register(newTestTool("b", ""))

	all := r.AllTools()
	if len(all) != 2 {
		t.Errorf("expected 2 tools, got %d", len(all))
	}
}

func TestToolRegistry_ToSlice(t *testing.T) {
	r := NewToolRegistry()
	r.Register(newTestTool("x", ""))
	r.Register(newTestTool("y", ""))

	slice := r.ToSlice()
	if len(slice) != 2 {
		t.Errorf("expected 2 tools, got %d", len(slice))
	}
}

func TestToolRegistry_Filter(t *testing.T) {
	r := NewToolRegistry()
	r.Register(newTestTool("search_web", ""), WithCategory("web"))
	r.Register(newTestTool("search_file", ""), WithCategory("file"))
	r.Register(newTestTool("read_file", ""), WithCategory("file"))

	filtered := r.Filter(func(tool Tool) bool {
		return tool.Name() == "search_file"
	})
	if len(filtered.AllTools()) != 1 {
		t.Errorf("expected 1 filtered tool, got %d", len(filtered.AllTools()))
	}
}

func TestToolRegistry_Merge(t *testing.T) {
	r1 := NewToolRegistry()
	r1.Register(newTestTool("a", ""), WithAlias("alias_a"))

	r2 := NewToolRegistry()
	r2.Register(newTestTool("b", ""), WithAlias("alias_b"))

	r1.Merge(r2)

	if _, ok := r1.Lookup("a"); !ok {
		t.Error("expected 'a' after merge")
	}
	if _, ok := r1.Lookup("b"); !ok {
		t.Error("expected 'b' after merge")
	}
	if _, ok := r1.Lookup("alias_b"); !ok {
		t.Error("expected 'alias_b' after merge")
	}
}

func TestToolRegistry_Unregister(t *testing.T) {
	r := NewToolRegistry()
	r.Register(newTestTool("temp", ""), WithAlias("t"), WithCategory("test"))

	r.Unregister("temp")

	if _, ok := r.Lookup("temp"); ok {
		t.Error("expected 'temp' to be removed")
	}
	if _, ok := r.Lookup("t"); ok {
		t.Error("expected alias 't' to be removed")
	}
	if len(r.LookupByCategory("test")) != 0 {
		t.Error("expected category 'test' to be empty")
	}
}

func TestToolRegistry_MustLookup(t *testing.T) {
	r := NewToolRegistry()
	r.Register(newTestTool("safe", ""))

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for missing tool")
		}
	}()
	r.MustLookup("nonexistent")
}

func TestToolRegistry_ConcurrentAccess(t *testing.T) {
	r := NewToolRegistry()
	r.Register(newTestTool("concurrent_tool", ""))

	done := make(chan struct{}, 2)
	go func() {
		for i := 0; i < 100; i++ {
			r.Lookup("concurrent_tool")
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 100; i++ {
			r.Register(newTestTool("other", ""))
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}

func TestToolToSliceForReActConfig(t *testing.T) {
	r := NewToolRegistry()
	r.Register(newTestTool("t1", ""))
	r.Register(newTestTool("t2", ""))

	cfg := &ReActConfig[Message]{
		Tools: r.ToSlice(),
	}
	if len(cfg.Tools) != 2 {
		t.Errorf("expected 2 tools in config, got %d", len(cfg.Tools))
	}
}

func TestToolRegistry_LookupPanicsForTool(t *testing.T) {
	// Non-existent tool via MustLookup
	r := NewToolRegistry()
	r.Register(&testPanicTool{name: "good"})

	// Should not panic
	_ = r.MustLookup("good")
}

type testPanicTool struct{ name string }

func (t *testPanicTool) Name() string { return t.name }
func (t *testPanicTool) Description() string { return "" }
func (t *testPanicTool) Invoke(ctx context.Context, s string, opts ...ToolOption) (string, error) { return "", nil }
func (t *testPanicTool) Stream(ctx context.Context, s string, opts ...ToolOption) (*schema.StreamReader[string], error) {
	return schema.StreamReaderFromArray([]string{""}), nil
}
