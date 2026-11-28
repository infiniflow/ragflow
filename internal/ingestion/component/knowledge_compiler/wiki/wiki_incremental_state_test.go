package wiki

import (
	"encoding/json"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestBuildActiveMapStateRecordsCurrentConceptPolicy(t *testing.T) {
	for _, topicMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "entity", true: "topic"}[topicMode], func(t *testing.T) {
			pipeline := &wikiPipeline{
				activeStateKey: "new-config-state",
				tenantID:       "tenant",
				datasetID:      "dataset",
				docID:          "document",
				deps:           common.Deps{WikiMapVersions: &memoryWikiMapVersionStore{}},
				param: common.Param{
					TemplateID:     "template",
					Plan:           &topicMode,
					TemplateConfig: map[string]any{"disable_concept_pages": true},
				},
			}
			state, err := pipeline.buildActiveMapState([]wikiPlanPage{{Slug: "entity/acme", PageType: "entity"}})
			if err != nil {
				t.Fatal(err)
			}
			var snapshot map[string]any
			if err := json.Unmarshal(state.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot["template_id"] != "template" || snapshot["concept_pages_disabled"] != !topicMode {
				t.Fatalf("snapshot = %s, want current template and effective entity-mode policy", state.Payload)
			}
			if len(snapshot["plan"].([]any)) != 1 || state.Key != "new-config-state" {
				t.Fatalf("snapshot lost its complete plan or new namespace: %+v", state)
			}
		})
	}
}

func TestConceptPolicyChangeCreatesNewActiveState(t *testing.T) {
	store := &claimGuidanceCacheStore{activeStates: map[string][]byte{}}
	pipeline := &wikiPipeline{
		ctx:       t.Context(),
		tenantID:  "tenant-1",
		datasetID: "kb-1",
		docID:     "doc-1",
		deps:      common.Deps{WikiMapVersions: store},
		param: common.Param{
			TemplateID:     "template",
			TemplateConfig: map[string]any{"disable_concept_pages": false},
		},
	}
	if err := pipeline.runVersionedMap(); err != nil {
		t.Fatal(err)
	}
	oldState, err := pipeline.buildActiveMapState([]wikiPlanPage{{Slug: "concept/old", PageType: "concept"}})
	if err != nil {
		t.Fatal(err)
	}
	store.activeStates[oldState.Key] = oldState.Payload
	pipeline.param.TemplateConfig["disable_concept_pages"] = true
	if err := pipeline.runVersionedMap(); err != nil {
		t.Fatal(err)
	}
	if pipeline.activeStateKey == oldState.Key || len(pipeline.previousActiveState.Plan) != 0 {
		t.Fatal("concept policy change did not create a new active-state namespace")
	}
	newState, err := pipeline.buildActiveMapState(nil)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(newState.Payload, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["template_id"] != "template" || snapshot["concept_pages_disabled"] != true {
		t.Fatalf("new namespace lost ownership or policy: %s", newState.Payload)
	}
}

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
