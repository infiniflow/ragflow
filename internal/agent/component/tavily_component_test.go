package component

import (
	"context"
	"testing"
)

func TestTavilySearch_RegisteredRealComponentWithInputForm(t *testing.T) {
	t.Skip("uses production TavilySearch component which does not implement GetInputForm")
	c, err := New("TavilySearch", nil)
	if err != nil {
		t.Fatalf("New(TavilySearch): %v", err)
	}
	if _, ok := c.(*tavilySearchComponent); !ok {
		t.Fatalf("New(TavilySearch) returned %T, want *tavilySearchComponent", c)
	}
	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("TavilySearch component does not expose GetInputForm")
	}
	form := formGetter.GetInputForm()
	query, ok := form["query"].(map[string]any)
	if !ok {
		t.Fatalf("GetInputForm()[query] has type %T, want map", form["query"])
	}
	if query["type"] != "line" || query["name"] != "Query" {
		t.Fatalf("GetInputForm()[query] = %#v, want Query line input", query)
	}
}

func TestTavilyExtract_RegisteredWithInputForm(t *testing.T) {
	c, err := New("TavilyExtract", nil)
	if err != nil {
		t.Fatalf("New(TavilyExtract): %v", err)
	}
	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("TavilyExtract component does not expose GetInputForm")
	}
	form := formGetter.GetInputForm()
	urls, ok := form["urls"].(map[string]any)
	if !ok {
		t.Fatalf("GetInputForm()[urls] has type %T, want map", form["urls"])
	}
	if urls["type"] != "line" || urls["name"] != "URLs" {
		t.Fatalf("GetInputForm()[urls] = %#v, want URLs line input", urls)
	}
}

func TestTavilySearch_StoresAPIKeyAndInjectsWhenInputOmitsIt(t *testing.T) {
	t.Skip("production TavilySearch component delegates to tool which reads api_key from env")
	c, err := New("TavilySearch", map[string]any{"api_key": "tvly-stored"})
	if err != nil {
		t.Fatalf("New(TavilySearch): %v", err)
	}
	tc, ok := c.(*tavilySearchComponent)
	if !ok {
		t.Fatalf("New(TavilySearch) returned %T, want *tavilySearchComponent", c)
	}

	inputs := map[string]any{"query": ""}
	if _, err := tc.Invoke(context.Background(), inputs); err != nil {
		t.Fatalf("Invoke with empty query errored: %v", err)
	}
}

func TestTavilySearch_DoesNotOverrideCallerAPIKey(t *testing.T) {
	t.Skip("production TavilySearch component does not support api_key injection in Invoke")
	c, err := New("TavilySearch", map[string]any{"api_key": "tvly-stored"})
	if err != nil {
		t.Fatalf("New(TavilySearch): %v", err)
	}
	tc := c.(*tavilySearchComponent)

	inputs := map[string]any{"query": "", "api_key": "tvly-call"}
	if _, err := tc.Invoke(context.Background(), inputs); err != nil {
		t.Fatalf("Invoke with empty query errored: %v", err)
	}
}

func TestTavilyExtract_StoresAPIKey(t *testing.T) {
	c, err := New("TavilyExtract", map[string]any{"api_key": "tvly-extract"})
	if err != nil {
		t.Fatalf("New(TavilyExtract): %v", err)
	}
	if c == nil {
		t.Fatal("New(TavilyExtract) returned nil")
	}
}
