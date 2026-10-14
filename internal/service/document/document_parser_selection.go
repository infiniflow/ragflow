package document

import (
	"context"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/service"
	"ragflow/internal/utility"

	"go.uber.org/zap"
)

// defaultDocumentParserID returns the parser that owns a document of the given
// file type, mirroring Python's FileService.get_parser
// (api/db/services/file_service.py:734-743): images and video are parsed by the
// picture pipeline, audio by audio, presentations by presentation and mail by
// email. Every other file type keeps the dataset's parser.
func defaultDocumentParserID(filetype utility.FileType, filename, fallback string) string {
	switch filetype {
	case utility.FileTypeVISUAL:
		return "picture"
	case utility.FileTypeAURAL:
		return "audio"
	}

	// The presentation and email extensions all classify as FileTypeDOC, so the
	// name decides (same name-based rule as Python's get_parser and the
	// document parser_id guard in document_dataset_update.go).
	name := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(name, ".ppt"),
		strings.HasSuffix(name, ".pptx"),
		strings.HasSuffix(name, ".pages"):
		return "presentation"
	case strings.HasSuffix(name, ".msg"),
		strings.HasSuffix(name, ".eml"):
		return "email"
	}
	return fallback
}

// resolveDocumentParser returns the parser_id and parser_config a newly created
// document should carry.
//
// A file type owned by a dedicated pipeline overrides the dataset's parser and
// the document is seeded with that pipeline's component params, so the row
// matches the parser it actually runs with: document parser_config drives both
// the parser-gap validation and the runtime override params. Without the config
// swap the document would keep another template's component ids and be reported
// as unsupported by its own file type.
//
// Canvas pipeline mode keeps the dataset parser untouched (parser_id is blanked
// by the caller). On a DSL failure the dataset parser and config are kept: a
// registry lookup must not fail an upload, and a half-switched row (new parser,
// old config) is exactly the inconsistent state this function exists to avoid.
func resolveDocumentParser(ctx context.Context, kb *entity.Knowledgebase, filename string, filetype utility.FileType, base entity.JSONMap) (string, entity.JSONMap) {
	if kb.PipelineID != nil {
		return kb.ParserID, base
	}
	parserID := defaultDocumentParserID(filetype, filename, kb.ParserID)
	if parserID == kb.ParserID || parserID == "" {
		return kb.ParserID, base
	}

	dslJSON, err := service.LoadPipelineDSL(ctx, false, parserID, nil)
	if err != nil {
		common.Warn("document: file-type parser DSL unavailable, keeping dataset parser",
			zap.String("parser_id", parserID), zap.String("filename", filename), zap.Error(err))
		return kb.ParserID, base
	}
	raw := map[string]interface{}(base)
	cleaned := pipelinepkg.BuildParserConfig(dslJSON, raw)
	// The dataset-level metadata group is parser-independent; keep it so the
	// target pipeline's Extractor node inherits the dataset setting instead of
	// falling back to disabled.
	if meta, ok := raw["metadata"]; ok {
		cleaned["metadata"] = meta
	}
	pipelinepkg.ApplyParentChildChunkerConfig(cleaned, raw)
	if tenant, tenantErr := dao.NewTenantDAO().GetByID(ctx, dao.DB, kb.TenantID); tenantErr == nil && tenant != nil {
		cleaned = service.ApplyComponentScopedParserConfig(cleaned, tenant.LLMID)
	}
	return parserID, cleaned
}
