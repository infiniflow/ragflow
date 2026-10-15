package document

import (
	"context"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	componentpkg "ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/schema"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/service"
	"ragflow/internal/utility"

	"go.uber.org/zap"
)

// generalParserID is the built-in catch-all ingestion pipeline. A file whose
// family the dataset's selection does not declare and which no dedicated
// pipeline owns falls back to it.
const generalParserID = "general"

// dedicatedDocumentParserID returns the pipeline that owns a document of the
// given file type, or "" when no pipeline owns it and the dataset's selection
// governs the file. Mirrors Python's FileService.get_parser
// (api/db/services/file_service.py:734-743): images and video are parsed by the
// picture pipeline, audio by audio, presentations by presentation and mail by
// email.
func dedicatedDocumentParserID(filetype utility.FileType, filename string) string {
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
	return ""
}

// familySupport is the three-state answer to "does this parser_config declare
// a file family".
type familySupport int

const (
	// familySupportYes: the Parser entry declares the family.
	familySupportYes familySupport = iota
	// familySupportNo: a Parser entry exists and does not declare the family.
	familySupportNo
	// familySupportUnknown: no Parser entry (legacy flat configs, canvases
	// without a Parser node), a malformed entry, or an unmappable family.
	familySupportUnknown
)

// parserConfigFamilySupport reports whether a parser_config declares the given
// Parser file family ("pdf", "text&code", "slides", ...).
//
// The Parser entry is the same declaration the web parser-gap validation reads
// (transformParserConfigSetups), so the backend resolves against the contract
// the front end validates. Both a dataset's built-in parser config and its
// canvas-derived config land in this shape; a config without a Parser entry
// leaves support undetermined.
func parserConfigFamilySupport(config entity.JSONMap, family string) familySupport {
	if family == "" {
		return familySupportUnknown
	}
	for cpnID, raw := range config {
		if !strings.HasPrefix(strings.ToLower(cpnID), "parser:") {
			continue
		}
		params, ok := raw.(map[string]any)
		if !ok {
			return familySupportUnknown
		}
		if _, declared := schema.FlattenLegacyParserSetups(params)[family]; declared {
			return familySupportYes
		}
		return familySupportNo
	}
	return familySupportUnknown
}

// selectionFamilySupport reports whether the dataset's selection declares the
// given file family.
//
// The dataset config is the declaration the web parser-gap validation reads, so
// a config that carries a Parser entry decides on its own. A config without one
// (legacy flat configs such as parser_id="naive", or a canvas without a Parser
// node) cannot answer from the config: a builtin selection still has its
// template, which is what the pipeline actually runs, while a canvas has no
// registry entry and stays undetermined.
func selectionFamilySupport(ctx context.Context, sel *service.ParserSelection, config entity.JSONMap, family string) familySupport {
	if family == "" {
		return familySupportUnknown
	}
	switch parserConfigFamilySupport(config, family) {
	case familySupportYes:
		return familySupportYes
	case familySupportNo:
		return familySupportNo
	}
	if sel != nil && sel.IsBuiltIn() {
		return builtinTemplateFamilySupport(ctx, sel.ParserID, family)
	}
	return familySupportUnknown
}

// builtinTemplateFamilySupport reads a builtin template's declared families.
// The registry resolves legacy aliases ("naive" -> "general"), so a dataset
// row storing a historical parser_id is judged by the template it actually
// runs. Undetermined when the template cannot be loaded or declares no Parser
// node.
func builtinTemplateFamilySupport(ctx context.Context, parserID, family string) familySupport {
	dslJSON, err := service.LoadPipelineDSL(ctx, false, parserID, nil)
	if err != nil {
		return familySupportUnknown
	}
	defaults, err := pipelinepkg.ComponentParamsDefaults(dslJSON)
	if err != nil {
		return familySupportUnknown
	}
	for cpnID, params := range defaults {
		if !strings.HasPrefix(strings.ToLower(cpnID), "parser:") {
			continue
		}
		if _, declared := schema.FlattenLegacyParserSetups(params)[family]; declared {
			return familySupportYes
		}
		return familySupportNo
	}
	return familySupportUnknown
}

// parserSelectionColumns maps a resolved parser selection onto the document
// row's (parser_id, pipeline_id) columns. A canvas selection leaves parser_id
// empty: parser_id is not applicable in canvas pipeline mode.
func parserSelectionColumns(sel *service.ParserSelection) (string, *string) {
	if sel != nil && sel.IsPipeline() {
		pipelineID := sel.PipelineID
		return "", &pipelineID
	}
	parserID := ""
	if sel != nil {
		parserID = sel.ParserID
	}
	return parserID, nil
}

// resolveDocumentParser returns the parser selection and parser_config a newly
// created document should carry.
//
// The dataset's selection (a built-in parser or a user canvas) governs every
// file family its config declares. A file the selection does not declare is
// handed to the built-in pipeline that owns its family (picture / audio /
// presentation / email), and failing that to the catch-all general pipeline, so
// the row always matches the pipeline that actually runs it: document
// parser_config drives both the parser-gap validation and the runtime override
// params, and a row left on a parser that cannot run its file would be reported
// as unsupported by its own type.
//
// A config without a Parser entry (legacy flat configs) is judged by the
// builtin template it runs; a canvas without a Parser node, and a filename
// whose extension maps to no family, leave the selection untouched: support
// cannot be determined there, and the user's choice is kept. On a DSL failure
// the selection and config are kept too: a registry lookup must not fail an
// upload, and a half-switched row (new parser, old config) is the inconsistent
// state this function exists to avoid.
func resolveDocumentParser(ctx context.Context, kb *entity.Knowledgebase, filename string, filetype utility.FileType, base entity.JSONMap) (*service.ParserSelection, entity.JSONMap) {
	selection := service.CurrentSelection(kb.ParserID, kb.PipelineID)
	family := componentpkg.ParserFileFamily(utility.GetFileExtension(filename))
	if selectionFamilySupport(ctx, selection, kb.ParserConfig, family) != familySupportNo {
		return selection, base
	}

	target := dedicatedDocumentParserID(filetype, filename)
	if target == "" {
		target = generalParserID
	}
	if rebuilt, ok := rebuildParserConfig(ctx, kb, target, filename, base); ok {
		return &service.ParserSelection{Mode: service.ParseModeBuiltIn, ParserID: target}, rebuilt
	}
	return selection, base
}

// rebuildParserConfig reseeds a document's parser_config from the target
// built-in pipeline's DSL. It reports false when the DSL cannot be loaded, so
// the caller keeps the dataset's selection instead of writing a half-switched
// row.
func rebuildParserConfig(ctx context.Context, kb *entity.Knowledgebase, target, filename string, base entity.JSONMap) (entity.JSONMap, bool) {
	dslJSON, err := service.LoadPipelineDSL(ctx, false, target, nil)
	if err != nil {
		common.Warn("document: target pipeline DSL unavailable, keeping dataset selection",
			zap.String("parser_id", target), zap.String("filename", filename), zap.Error(err))
		return nil, false
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
	return cleaned, true
}
