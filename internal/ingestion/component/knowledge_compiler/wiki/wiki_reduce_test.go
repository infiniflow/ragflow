package wiki

import "testing"

func TestReduceExtracts_MergesDuplicateEntitySlug(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{
		{Entities: []wikiEntity{{Name: "曹操", Type: "person", Aliases: []string{"孟德"}, SourceChunkIDs: []string{"c1"}}}},
		{Entities: []wikiEntity{{Name: "曹操", Type: "person", SourceChunkIDs: []string{"c2"}}}},
	})
	if len(reduced.Entities) != 1 {
		t.Fatalf("entities = %d, want 1", len(reduced.Entities))
	}
	if got := reduced.Entities[0].SourceChunkIDs; len(got) != 2 {
		t.Fatalf("source chunk ids = %#v, want both chunks", got)
	}
	if got := reduced.Entities[0].Aliases; len(got) != 1 || got[0] != "孟德" {
		t.Fatalf("aliases = %#v, want [孟德]", got)
	}
}

func TestReduceExtracts_DifferentEntityTypesKeepDifferentSlugs(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{
		{Entities: []wikiEntity{{Name: "苹果", Type: "fruit"}}},
		{Entities: []wikiEntity{{Name: "苹果", Type: "company"}}},
	})
	if len(reduced.Entities) != 2 {
		t.Fatalf("entities = %d, want 2", len(reduced.Entities))
	}
	if got := entityPageSlug(reduced.Entities[0].Name, reduced.Entities[0].Type); got == entityPageSlug(reduced.Entities[1].Name, reduced.Entities[1].Type) {
		t.Fatalf("different entity types have the same slug %q", got)
	}
}

func TestReduceExtracts_DoesNotMergeSimilarNames(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{
		{Entities: []wikiEntity{{Name: "Alpha", Type: "org"}}},
		{Entities: []wikiEntity{{Name: "Alpha Incorporated", Type: "org"}}},
	})
	if len(reduced.Entities) != 2 {
		t.Fatalf("entities = %d, want 2; REDUCE must not perform semantic merging", len(reduced.Entities))
	}
}

func TestReduceExtracts_MergesDuplicateRelationProvenance(t *testing.T) {
	reduced := reduceExtracts([]wikiExtract{
		{Relations: []wikiRelation{{From: "A", To: "B", Type: "knows", SourceChunkIDs: []string{"c1"}}}},
		{Relations: []wikiRelation{{From: "A", To: "B", Type: "knows", SourceChunkIDs: []string{"c2"}}}},
	})
	if len(reduced.Relations) != 1 || len(reduced.Relations[0].SourceChunkIDs) != 2 {
		t.Fatalf("relations = %#v, want one relation with both source chunks", reduced.Relations)
	}
}

func TestEntityPageSlugIncludesType(t *testing.T) {
	if got, want := entityPageSlug("曹操", "person"), "entity/person-曹操"; got != want {
		t.Fatalf("entityPageSlug = %q, want %q", got, want)
	}
	if got, want := entityPageSlug("曹操", ""), "entity/曹操"; got != want {
		t.Fatalf("entityPageSlug without type = %q, want %q", got, want)
	}
}
