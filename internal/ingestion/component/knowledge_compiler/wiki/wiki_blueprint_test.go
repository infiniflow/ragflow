package wiki

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// blueprintPipeline returns a refine pipeline whose chat stub records the REFINE
// system prompt and returns a page without a "See also" section.
func blueprintPipeline(parserConfig map[string]any, system *string) *wikiPipeline {
	p := refinePipeline()
	p.inputs.VariantSpecific = map[string]any{"parser_config": parserConfig}
	p.deps.Chat = chatFunc(func(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
		*system = req.SystemPrompt
		return &common.ChatResponse{Content: "# Alpha\n\nAlpha exists.\n"}, nil
	})
	return p
}

func TestRunRefinePage_CustomBlueprintReplacesDefaultExample(t *testing.T) {
	var system string
	p := blueprintPipeline(map[string]any{
		"use_blueprint": true,
		"instruction":   "Write a terse FAQ page.",
		"example":       "Q: what?\nA: this.",
	}, &system)
	if _, err := refineTestPage(p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(system, "Write a terse FAQ page.") || !strings.Contains(system, "Q: what?") {
		t.Fatalf("custom blueprint missing from REFINE system prompt: %q", system)
	}
	if strings.Contains(system, wikiRefineWriterExample) {
		t.Fatal("default example must be replaced by the custom blueprint")
	}
}

func TestRunRefinePage_EmptyBlueprintKeepsDefaultExample(t *testing.T) {
	for name, cfg := range map[string]map[string]any{
		"no config":          nil,
		"blueprint disabled": {"use_blueprint": false, "instruction": "ignored", "example": "ignored"},
		"built-in text only": {"instruction": "ignored", "example": "ignored"},
		"enabled but empty":  {"use_blueprint": true, "instruction": " ", "example": ""},
	} {
		t.Run(name, func(t *testing.T) {
			var system string
			p := blueprintPipeline(cfg, &system)
			if _, err := refineTestPage(p); err != nil {
				t.Fatal(err)
			}
			if system != buildWikiRefineWriterSystem("") || !strings.Contains(system, wikiRefineWriterExample) {
				t.Fatalf("default REFINE system prompt changed: %q", system)
			}
		})
	}
}

func TestRunRefinePage_SeeAlsoOffKeepsOutlinksInMetadata(t *testing.T) {
	plan := func(p *wikiPipeline) (wikiPageResult, error) {
		return p.runRefinePage(wikiPlanPage{
			Action: "CREATE", Slug: "entity/alpha", Title: "Alpha", PageType: "entity",
			EntityNames: []string{"Alpha"}, RelatedKB: []string{"entity/beta"},
		}, []string{"entity/alpha", "entity/beta"},
			map[string]string{"entity/alpha": "Alpha", "entity/beta": "Beta"},
			map[string]string{"entity/alpha": "entity", "entity/beta": "entity"},
			buildWikiEntityLookup(nil), buildWikiConceptLookup(nil))
	}
	var system string
	on := blueprintPipeline(nil, &system)
	onPage, err := plan(on)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(onPage.Content, "## See also") {
		t.Fatalf("default must append See also: %q", onPage.Content)
	}
	off := blueprintPipeline(map[string]any{"see_also": false}, &system)
	offPage, err := plan(off)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(offPage.Content), "see also") {
		t.Fatalf("see_also=false must not append a See also section: %q", offPage.Content)
	}
	if strings.Join(offPage.Outlinks, ",") != strings.Join(onPage.Outlinks, ",") || len(offPage.Outlinks) == 0 {
		t.Fatalf("outlinks must stay in metadata: on=%v off=%v", onPage.Outlinks, offPage.Outlinks)
	}
}
