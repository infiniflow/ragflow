//go:build manual

package agentic_rag

import (
	"context"
	"os"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/server"
)

// TestCompiledServeSmoke_Manual is a read-only probe against a live document
// engine: it takes one real chunk id (a chunk that a claim row points at through
// source_chunk_ids) and asks the compiled-serve path to replace it with the
// claim. It writes nothing.
//
// Run with:
//
//	SMOKE_TENANT=<tenant> SMOKE_CHUNK=<chunk id> SMOKE_DOC=<doc id> \
//	  bash build.sh --test-manual -run TestCompiledServeSmoke_Manual ./internal/agentic_rag
func TestCompiledServeSmoke_Manual(t *testing.T) {
	tenantID := os.Getenv("SMOKE_TENANT")
	chunkID := os.Getenv("SMOKE_CHUNK")
	docID := os.Getenv("SMOKE_DOC")
	if tenantID == "" || chunkID == "" {
		t.Fatal("SMOKE_TENANT and SMOKE_CHUNK are required")
	}

	t.Setenv("RAGFLOW_COMPILED_SERVE", "only")
	t.Setenv("RAGFLOW_COMPILED_SERVE_KINDS", "tree")
	t.Setenv("RAGFLOW_COMPILED_SERVE_TYPES", "claim")
	t.Setenv("RAGFLOW_COMPILED_SERVE_TOP_N", "5")

	if err := common.InitLogger("info", common.FileOutput{}, ""); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	if err := server.Init(""); err != nil {
		t.Fatalf("init config: %v", err)
	}
	if err := engine.InitDocEngine(context.Background()); err != nil {
		t.Fatalf("init doc engine: %v", err)
	}

	chunks := []map[string]interface{}{{
		"id":                  chunkID,
		"doc_id":              docID,
		"content_with_weight": "raw chunk body the compiled path should replace",
	}}

	out := CompiledServeChunks(context.Background(), tenantID, chunks, []string{})
	if len(out) == 0 {
		t.Fatalf("compiled serve returned nothing for chunk %s", chunkID)
	}
	for i, c := range out {
		body := strings.TrimSpace(smokeString(c["content_with_weight"]))
		if body == "" {
			body = strings.TrimSpace(smokeString(c["content"]))
		}
		t.Logf("served[%d] keys=%d body=%q", i, len(c), smokeTruncate(body, 300))
	}
	if len(out) == 1 && smokeString(out[0]["content_with_weight"]) == chunks[0]["content_with_weight"] {
		t.Fatalf("compiled serve fell back to the raw chunk; no claim product was served")
	}
}

func smokeString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func smokeTruncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
