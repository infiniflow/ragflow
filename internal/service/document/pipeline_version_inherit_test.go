package document

import (
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// T8: a document uploaded into a dataflow knowledgebase inherits the KB's
// pinned pipeline_version_id (sticky snapshot, design section 5/B4). The
// document locks to the version the KB was bound to at upload time, so later
// version publishes do not change already-ingested files.
func TestNewDatasetDocument_InheritsPipelineVersionID(t *testing.T) {
	pid := "canvas-1"
	ver := "v1"
	kb := &entity.Knowledgebase{
		ID:                "kb-1",
		PipelineID:        &pid,
		PipelineVersionID: &ver,
	}
	sel := &service.ParserSelection{Mode: service.ParseModePipeline, PipelineID: pid}
	doc := (&DocumentService{}).newDatasetDocument(kb, "tenant-1", "f.pdf", "loc", "pdf", sel, entity.JSONMap{}, "local", 10, nil)
	if doc.PipelineID == nil || *doc.PipelineID != "canvas-1" {
		t.Fatalf("PipelineID = %v, want canvas-1", doc.PipelineID)
	}
	if doc.PipelineVersionID == nil || *doc.PipelineVersionID != "v1" {
		t.Fatalf("document did not inherit kb pipeline_version_id: %v", doc.PipelineVersionID)
	}
}

// T8b: a built-in (non-pipeline) knowledgebase must not carry a
// pipeline_version_id on its documents (design section 5, built-in unaffected).
func TestNewDatasetDocument_BuiltinHasNoVersion(t *testing.T) {
	kb := &entity.Knowledgebase{ID: "kb-2"}
	sel := &service.ParserSelection{Mode: service.ParseModeBuiltIn, ParserID: "general"}
	doc := (&DocumentService{}).newDatasetDocument(kb, "tenant-1", "f.pdf", "loc", "pdf", sel, entity.JSONMap{}, "local", 10, nil)
	if doc.PipelineVersionID != nil {
		t.Fatalf("built-in document must not carry pipeline_version_id: %v", doc.PipelineVersionID)
	}
}
