package component

import (
	"context"
	"testing"
)

func TestTavilySearch_RegisteredRealComponentWithInputForm(t *testing.T) {
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
	if _, ok := c.(*tavilyExtractComponent); !ok {
		t.Fatalf("New(TavilyExtract) returned %T, want *tavilyExtractComponent", c)
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
	c, err := New("TavilySearch", map[string]any{"api_key": "tvly-stored"})
	if err != nil {
		t.Fatalf("New(TavilySearch): %v", err)
	}
	tc, ok := c.(*tavilySearchComponent)
	if !ok {
		t.Fatalf("New(TavilySearch) returned %T, want *tavilySearchComponent", c)
	}
	if tc.apiKey != "tvly-stored" {
		t.Fatalf("stored apiKey = %q, want tvly-stored", tc.apiKey)
	}

	inputs := map[string]any{"query": ""}
	if _, err := tc.Invoke(context.Background(), inputs); err != nil {
		t.Fatalf("Invoke with empty query errored: %v", err)
	}
	if got := inputs["api_key"]; got != "tvly-stored" {
		t.Fatalf("injected api_key = %v, want tvly-stored", got)
	}
}

func TestTavilySearch_DoesNotOverrideCallerAPIKey(t *testing.T) {
	c, err := New("TavilySearch", map[string]any{"api_key": "tvly-stored"})
	if err != nil {
		t.Fatalf("New(TavilySearch): %v", err)
	}
	tc := c.(*tavilySearchComponent)

	inputs := map[string]any{"query": "", "api_key": "tvly-call"}
	if _, err := tc.Invoke(context.Background(), inputs); err != nil {
		t.Fatalf("Invoke with empty query errored: %v", err)
	}
	if got := inputs["api_key"]; got != "tvly-call" {
		t.Fatalf("api_key = %v, want caller key", got)
	}
}

func TestTavilyExtract_StoresAPIKey(t *testing.T) {
	c, err := New("TavilyExtract", map[string]any{"api_key": "tvly-extract"})
	if err != nil {
		t.Fatalf("New(TavilyExtract): %v", err)
	}
	tc, ok := c.(*tavilyExtractComponent)
	if !ok {
		t.Fatalf("New(TavilyExtract) returned %T, want *tavilyExtractComponent", c)
	}
	if tc.apiKey != "tvly-extract" {
		t.Fatalf("stored apiKey = %q, want tvly-extract", tc.apiKey)
	}
}
