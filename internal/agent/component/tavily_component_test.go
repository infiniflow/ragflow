package component

import "testing"

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
