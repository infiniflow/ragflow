package component

import (
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/ingestion/task/indexdoc"
)

// TestRepro_FieldNameMetadata_AggregatesMap guards the unified contract:
// field_name="metadata" (the pipeline/canvas way to produce document metadata)
// writes a parsed map into ck["metadata"] — not a raw string — so the
// aggregation layer, which is strict on map, merges it into document metadata.
func TestRepro_FieldNameMetadata_AggregatesMap(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: `{"category":"finance","region":"east"}`},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "metadata",
		LLMID:     "gpt-4o-mini",
		Prompt:    "Extract metadata as JSON:",
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "first text"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := out["chunks"].([]map[string]any)
	meta, ok := chunks[0]["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("chunk[0].metadata = %T, want map[string]any (field_name=metadata must produce a map, not a string)", chunks[0]["metadata"])
	}
	if meta["category"] != "finance" || meta["region"] != "east" {
		t.Errorf("metadata = %v, want category=finance region=east", meta)
	}

	// Feed through the aggregation path, exactly as the executor does.
	metadata, err := indexdoc.ProcessChunksForPipeline(chunks, "doc-1", "doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	if metadata["category"] != "finance" {
		t.Errorf("category = %v, want finance", metadata["category"])
	}
	if metadata["region"] != "east" {
		t.Errorf("region = %v, want east", metadata["region"])
	}
}

// TestRepro_BothEnabled_MergesMetadata guards the unified contract: when both
// enable_metadata (auto, schema-driven) and field_name="metadata" (pipeline
// custom) are configured, the extractor MERGES them into one ck["metadata"]
// map instead of the field_name result overwriting the auto-metadata. Both
// contribute to the document metadata.
func TestRepro_BothEnabled_MergesMetadata(t *testing.T) {
	withStubChatInvoker(t,
		// call 1: enable_metadata (runAutoExtractions)
		stubResponse{Content: `{"category":"finance","region":"east"}`},
		// call 2: field_name="metadata" primary extraction
		stubResponse{Content: `{"author":"zhang","date":"2024-01-01"}`},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:      "metadata",
		LLMID:          "gpt-4o-mini",
		Prompt:         "Extract more metadata as JSON:",
		EnableMetadata: 1,
		Metadata: []common.MetadataFieldDef{
			{Key: "category", Type: "string"},
			{Key: "region", Type: "string"},
		},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "content"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := out["chunks"].([]map[string]any)
	meta, ok := chunks[0]["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("chunk[0].metadata = %T, want merged map; auto-metadata was overwritten", chunks[0]["metadata"])
	}
	for _, k := range []string{"category", "region", "author", "date"} {
		if _, has := meta[k]; !has {
			t.Errorf("merged metadata missing %q: %v", k, meta)
		}
	}

	metadata, err := indexdoc.ProcessChunksForPipeline(chunks, "doc-1", "doc.pdf", time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}
	for _, k := range []string{"category", "region", "author", "date"} {
		if _, has := metadata[k]; !has {
			t.Errorf("aggregated doc metadata missing %q: %v", k, metadata)
		}
	}
}

// TestRepro_OverlapFieldNameWins pins the overlap rule: when enable_metadata
// and field_name="metadata" extract the same key, the field_name (custom,
// later) result wins.
func TestRepro_OverlapFieldNameWins(t *testing.T) {
	withStubChatInvoker(t,
		// call 1: enable_metadata
		stubResponse{Content: `{"category":"finance","region":"east"}`},
		// call 2: field_name="metadata"
		stubResponse{Content: `{"category":"law","author":"X"}`},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:      "metadata",
		LLMID:          "gpt-4o-mini",
		Prompt:         "Extract metadata as JSON:",
		EnableMetadata: 1,
		Metadata: []common.MetadataFieldDef{
			{Key: "category", Type: "string"},
			{Key: "region", Type: "string"},
		},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "content"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := out["chunks"].([]map[string]any)
	meta := chunks[0]["metadata"].(map[string]any)
	if meta["category"] != "law" {
		t.Errorf("category = %v, want law (field_name wins on overlap)", meta["category"])
	}
	if meta["region"] != "east" {
		t.Errorf("region = %v, want east (auto field preserved)", meta["region"])
	}
}

// TestRepro_FieldNameNonJSON_KeepsEnableMetadata verifies a non-JSON
// field_name="metadata" result does not clobber the enable_metadata map and
// never writes a raw string.
func TestRepro_FieldNameNonJSON_KeepsEnableMetadata(t *testing.T) {
	withStubChatInvoker(t,
		// call 1: enable_metadata
		stubResponse{Content: `{"category":"finance"}`},
		// call 2: field_name="metadata" -> non-JSON
		stubResponse{Content: "could not find metadata"},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName:      "metadata",
		LLMID:          "gpt-4o-mini",
		Prompt:         "Extract metadata as JSON:",
		EnableMetadata: 1,
		Metadata: []common.MetadataFieldDef{
			{Key: "category", Type: "string"},
		},
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"chunks": []map[string]any{{"text": "content"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := out["chunks"].([]map[string]any)
	meta, ok := chunks[0]["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("chunk[0].metadata = %T, want map (auto-metadata preserved, no string written)", chunks[0]["metadata"])
	}
	if meta["category"] != "finance" {
		t.Errorf("category = %v, want finance (enable_metadata map preserved)", meta["category"])
	}
}

// TestRepro_NoChunksMetadata_RoutesThroughMap guards the zero-chunk fast path:
// field_name="metadata" with no input chunks must route through the same
// parse-and-merge as the chunked path, writing a map (not a raw string) so the
// strict aggregation layer does not drop the document metadata.
func TestRepro_NoChunksMetadata_RoutesThroughMap(t *testing.T) {
	withStubChatInvoker(t,
		stubResponse{Content: `{"category":"finance","region":"east"}`},
	)
	c := &ExtractorComponent{Param: schema.ExtractorParam{
		FieldName: "metadata",
		LLMID:     "gpt-4o-mini",
		Prompt:    "Extract metadata as JSON:",
	}}
	out, err := c.Invoke(t.Context(), nil, map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("chunks len = %d, want 1", len(chunks))
	}
	meta, ok := chunks[0]["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("chunk[0].metadata = %T, want map[string]any (zero-chunk path must not write a raw string)", chunks[0]["metadata"])
	}
	if meta["category"] != "finance" || meta["region"] != "east" {
		t.Errorf("metadata = %v, want category=finance region=east", meta)
	}
}
