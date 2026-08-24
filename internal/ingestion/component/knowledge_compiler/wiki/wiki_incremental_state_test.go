package wiki

import "testing"

func TestSelectAffectedPagesIncludesOldAndNewMembership(t *testing.T) {
	pipeline := &wikiPipeline{
		incremental: true,
		mapChanged:  true,
		affectedTerms: map[string]struct{}{
			"alpha": {},
		},
		affectedPageSlugs: map[string]struct{}{},
		previousActiveState: wikiMapActiveSnapshot{
			Chunks: map[string]wikiMapActiveChunk{"chunk-1": {Key: "old"}},
			Plan: []wikiPlanPage{
				{Slug: "topic/old", PageType: "topic", EntityNames: []string{"Alpha"}},
				{Slug: "entity/stable", PageType: "entity", EntityNames: []string{"Stable"}},
			},
		},
	}
	current := []wikiPlanPage{
		{Slug: "topic/new", PageType: "topic", EntityNames: []string{"Alpha"}},
		{Slug: "entity/stable", PageType: "entity", EntityNames: []string{"Stable"}},
	}

	pipeline.selectAffectedPages(current)

	if _, ok := pipeline.affectedPageSlugs["topic/old"]; !ok {
		t.Fatal("old membership page was not affected")
	}
	if _, ok := pipeline.affectedPageSlugs["topic/new"]; !ok {
		t.Fatal("new membership page was not affected")
	}
	if _, ok := pipeline.affectedPageSlugs["entity/stable"]; ok {
		t.Fatal("unrelated stable page was affected")
	}
	if len(pipeline.removedPageSlugs) != 1 || pipeline.removedPageSlugs[0] != "topic/old" {
		t.Fatalf("removed pages = %#v, want topic/old", pipeline.removedPageSlugs)
	}
}

func TestSelectAffectedPagesNoChunkDeltaIsNoOp(t *testing.T) {
	pipeline := &wikiPipeline{
		incremental:       true,
		mapChanged:        false,
		affectedPageSlugs: map[string]struct{}{},
		previousActiveState: wikiMapActiveSnapshot{
			Chunks: map[string]wikiMapActiveChunk{"chunk-1": {Key: "same"}},
		},
	}
	pipeline.selectAffectedPages([]wikiPlanPage{{Slug: "entity/alpha", EntityNames: []string{"Alpha"}}})
	if len(pipeline.affectedPageSlugs) != 0 {
		t.Fatalf("affected pages = %#v, want none", pipeline.affectedPageSlugs)
	}
}

func TestSelectAffectedPagesMatchesOnlyChangedTopic(t *testing.T) {
	pipeline := &wikiPipeline{
		incremental: true,
		mapChanged:  true,
		affectedTerms: map[string]struct{}{
			"history": {},
		},
		affectedPageSlugs: map[string]struct{}{},
		previousActiveState: wikiMapActiveSnapshot{
			Chunks: map[string]wikiMapActiveChunk{"chunk-1": {Key: "old"}},
			Plan: []wikiPlanPage{
				{Slug: "topic/history", PageType: "topic", Topic: "History"},
				{Slug: "topic/science", PageType: "topic", Topic: "Science"},
			},
		},
	}
	current := append([]wikiPlanPage(nil), pipeline.previousActiveState.Plan...)

	pipeline.selectAffectedPages(current)

	if _, ok := pipeline.affectedPageSlugs["topic/history"]; !ok {
		t.Fatal("changed topic page was not selected")
	}
	if _, ok := pipeline.affectedPageSlugs["topic/science"]; ok {
		t.Fatal("unrelated topic page was selected")
	}
}
