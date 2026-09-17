//go:build integration

package pipeline

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The table template's real chain (File -> Parser -> TableChunker -> Tokenizer)
// must emit one chunk per row with the parser's structured metadata, and the
// parser's discovered column names must be reachable from the terminal payload.
//
// That second half is not cosmetic: the terminal component emits only its own
// chunks (see globals.GlobalMetadataKeys), so the column names live solely in
// the run state snapshot. Reading the payload's top level instead silently
// disabled column discovery — the parser's names never reached the document or
// the dataset's field_map.
func TestPipelineRun_TemplateTable_RealComponents(t *testing.T) {
	RequireTokenizerPool(t)

	templatePath := filepath.Join(repoRootFromPipelineTest(t), "internal", "ingestion", "pipeline", "template", "ingestion_pipeline_table.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
	if len(terminalIDs) != 1 || !strings.HasPrefix(terminalIDs[0], "Tokenizer:") {
		t.Fatalf("terminal ids = %v, want a single Tokenizer", terminalIDs)
	}

	mem := withRealTemplateDeps(t)

	const (
		bucket   = "test-bucket"
		path     = "fixtures/template-table.csv"
		filename = "template-table.csv"
	)
	content := "Title,Country,Internal\nDoc A,Turkey,42\nDoc B,Greece,7\n"
	docID := seedTemplateDocument(t, mem, filename, bucket, path, content)

	pipe, err := NewPipelineFromDSL(templateBytes, "template-table-real")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	attachFixedEmbedderFactory(t, pipe)
	out, err := pipe.Run(t.Context(), map[string]any{
		"doc_id": docID,
		"kb_id":  "test-kb",
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])

	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok || len(chunks) != 2 {
		t.Fatalf("chunks = %T/%v, want 2 rows", payload["chunks"], payload["chunks"])
	}
	wantRows := []map[string]string{
		{"Title": "Doc A", "Country": "Turkey", "Internal": "42"},
		{"Title": "Doc B", "Country": "Greece", "Internal": "7"},
	}
	for i, want := range wantRows {
		chunkData, ok := chunks[i]["chunk_data"].(map[string]any)
		if !ok {
			t.Fatalf("chunks[%d] lost chunk_data in the pipeline: %v", i, chunks[i])
		}
		for column, value := range want {
			if got := chunkData[column]; got != value {
				t.Errorf("chunks[%d].chunk_data[%s] = %v, want %v", i, column, got, value)
			}
			if text, _ := chunks[i]["text"].(string); !strings.Contains(text, "- "+column+": "+value) {
				t.Errorf("chunks[%d].text missing %s: %q", i, column, text)
			}
		}
	}

	// The terminal payload itself carries no file map — this is the shape the
	// executor must cope with when it reads the discovered columns.
	if _, ok := payload["file"]; ok {
		t.Fatalf("terminal payload unexpectedly carries a file map: %v", payload["file"])
	}
	state := stateFromRunOutput(t, out)
	var discovered []string
	for cpnID, cpnState := range state {
		if !strings.HasPrefix(cpnID, "Parser:") {
			continue
		}
		fileMap, ok := cpnState["file"].(map[string]any)
		if !ok {
			continue
		}
		switch names := fileMap["table_column_names"].(type) {
		case []string:
			discovered = names
		case []interface{}:
			discovered = make([]string, 0, len(names))
			for _, n := range names {
				discovered = append(discovered, n.(string))
			}
		}
	}
	if want := []string{"Title", "Country", "Internal"}; !reflect.DeepEqual(discovered, want) {
		t.Fatalf("parser state table_column_names = %v, want %v", discovered, want)
	}
}
