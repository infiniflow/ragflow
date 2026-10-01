package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/service"

	"github.com/google/uuid"
)

// keepDatasetOrderTerms narrows the requested terms to the columns the dataset
// list has always accepted, which is a smaller set than the knowledge base row
// exposes. A list with nothing left falls back to create_time in the first
// requested direction, which is what an unrecognised single name did.
func keepDatasetOrderTerms(terms []dao.OrderTerm) []dao.OrderTerm {
	kept := make([]dao.OrderTerm, 0, len(terms))
	for _, term := range terms {
		column := strings.TrimSpace(term.Column)
		if _, ok := datasetAllowedOrderByFields[column]; ok {
			kept = append(kept, dao.OrderTerm{Column: column, Desc: term.Desc})
		}
	}
	if len(kept) == 0 {
		return []dao.OrderTerm{{Column: "create_time", Desc: len(terms) > 0 && terms[0].Desc}}
	}
	return kept
}

// Package-level vars and constants used by the dataset service.
var (
	datasetSupportedAvatarMIMETypes = map[string]struct{}{
		"image/jpeg": {},
		"image/png":  {},
	}
	datasetAllowedOrderByFields = map[string]struct{}{
		"create_time": {},
		"update_time": {},
	}
	datasetAllowedMetadataTypes = map[string]struct{}{
		"string": {},
		"list":   {},
		"time":   {},
		"number": {},
	}
	validIndexTypes        = []string{"graph", "raptor", "mindmap"}
	indexTypeToTaskType    = map[string]string{"graph": "graphrag", "raptor": "raptor", "mindmap": "mindmap"}
	indexTypeToDisplayName = map[string]string{"graph": "Graph", "raptor": "RAPTOR", "mindmap": "Mindmap"}
)

const (
	maximumTaskPageNumber    = int64(100000000)
	serverQueueNamePrefix    = "te"
	defaultEmbeddingCheckNum = 5

	graphPhaseResolutionDone = "resolution_done"
	graphPhaseCommunityDone  = "community_done"
)

// canonicalDatasetParserID resolves a parser ID to its canonical builtin ID.
// The registry retains legacy aliases such as naive -> general for old clients.
func canonicalDatasetParserID(parserID string) (string, error) {
	if parserID == "knowledge_graph" {
		return parserID, nil
	}
	registry, err := pipelinepkg.DefaultRegistry()
	if err != nil || registry == nil {
		return "", errors.New("parser_id validation unavailable: builtin pipeline registry not loaded")
	}
	template, ok := registry.Get(parserID)
	if ok {
		return template.ParserID, nil
	}
	return "", parserIDError()
}

// validateParserID validates parser_id against the built-in pipeline registry.
func validateParserID(parserID string) error {
	_, err := canonicalDatasetParserID(parserID)
	return err
}

// datasetParserIDForResponse returns the canonical parser ID when a legacy
// persisted value remains resolvable. Unknown stored values are preserved.
func datasetParserIDForResponse(parserID string) string {
	canonicalID, err := canonicalDatasetParserID(parserID)
	if err != nil {
		return parserID
	}
	return canonicalID
}

func parserIDError() error {
	registry, err := pipelinepkg.DefaultRegistry()
	if err != nil || registry == nil {
		return errors.New("invalid parser_id")
	}
	refs := registry.Refs()
	switch len(refs) {
	case 0:
		return errors.New("invalid parser_id")
	case 1:
		return fmt.Errorf("input should be '%s'", refs[0])
	default:
		return fmt.Errorf("input should be %s or '%s'", quoteList(refs[:len(refs)-1]), refs[len(refs)-1])
	}
}

func quoteList(items []string) string {
	quoted := make([]string, len(items))
	for i, v := range items {
		quoted[i] = "'" + v + "'"
	}
	return strings.Join(quoted, ", ")
}

func validateDatasetAvatar(avatar string) error {
	if !strings.Contains(avatar, ",") {
		return errors.New("missing MIME prefix. Expected format: data:<mime>;base64,<data>")
	}
	prefix, _, _ := strings.Cut(avatar, ",")
	if !strings.HasPrefix(prefix, "data:") {
		return errors.New("invalid MIME prefix format. Must start with 'data:'")
	}
	mimeType, _, _ := strings.Cut(strings.TrimPrefix(prefix, "data:"), ";")
	if _, ok := datasetSupportedAvatarMIMETypes[mimeType]; !ok {
		return errors.New("unsupported MIME type. Allowed: [image/jpeg image/png]")
	}
	return nil
}

func isHexID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

func validateDatasetEmbeddingModel(embeddingModel string) error {
	if isHexID(embeddingModel) {
		return nil
	}

	if !strings.Contains(embeddingModel, "@") {
		return errors.New("embedding model identifier must follow <model_name>@<provider> format")
	}

	parts := strings.SplitN(embeddingModel, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("both model_name and provider must be non-empty strings")
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return errors.New("both model_name and provider must be non-empty strings")
	}
	return nil
}

func normalizeDatasetPipelineID(pipelineID string) (*string, error) {
	pipelineID = strings.TrimSpace(pipelineID)
	if pipelineID == "" {
		return nil, nil
	}
	if len(pipelineID) != 32 {
		return nil, errors.New("pipeline_id must be 32 hex characters")
	}
	for _, char := range pipelineID {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return nil, errors.New("pipeline_id must be hexadecimal")
		}
	}
	normalized := strings.ToLower(pipelineID)
	return &normalized, nil
}

func validateDatasetParserConfigSize(parserConfig map[string]interface{}) error {
	if len(parserConfig) == 0 {
		return nil
	}
	data, err := json.Marshal(parserConfig)
	if err != nil {
		return errors.New("parser_config must be valid JSON")
	}
	if len(data) > 65535 {
		return fmt.Errorf("Parser config exceeds size limit (max 65,535 characters). Current size: %d", len(data))
	}
	return nil
}

// DropUnscopedParserConfigKeys removes top-level parser_config keys that are
// not component-scoped (i.e. do not contain ":"), since the Go backend only
// consumes keys keyed by a component id (e.g. "Parser:abc123" or
// "Extractor:AutoExtractDefault"). Flat keys are legacy transport artifacts
// that downstream consumers (CleanComponentParams) never read, so they are
// dropped rather than rejected. Only top-level keys are affected; a key nested
// inside a component node (e.g. "Extractor:AutoExtractDefault.metadata") keeps
// its existing name. It mutates the provided map in place and returns the names
// of the dropped keys so callers can log the silent drop. A nil or empty map is
// returned untouched with a nil result.
func DropUnscopedParserConfigKeys(parserConfig map[string]any) []string {
	if len(parserConfig) == 0 {
		return nil
	}
	var dropped []string
	for key := range parserConfig {
		if !strings.Contains(key, ":") {
			dropped = append(dropped, key)
		}
	}
	for _, key := range dropped {
		delete(parserConfig, key)
	}
	return dropped
}

// ValidateParserConfig validates the shared REST parser_config schema. Flat
// (non-component-scoped) keys are no longer rejected: they are silently dropped
// because downstream consumers never read them. The size limit is still
// enforced. It returns the names of the dropped (unscoped) keys so the caller
// can log the silent drop; this is the single drop point shared by every entry
// path, so callers should not call DropUnscopedParserConfigKeys again.
func ValidateParserConfig(parserConfig map[string]interface{}) ([]string, error) {
	dropped := DropUnscopedParserConfigKeys(parserConfig)
	return dropped, validateDatasetParserConfigSize(parserConfig)
}

// ValidateDocumentParserConfig validates the parser_config attached to a
// document. Documents follow the same component-scoped contract as datasets:
// every key must be scoped under a node id (e.g. "Extractor:AutoExtractDefault"
// or "GeneralChunker:SixApplesFall"). A document's Extractor/GeneralChunker
// nodes come from the same pipeline DSL as the dataset, so flat keys are dropped
// (not kept) and the size limit is enforced. It returns the dropped key names
// for logging, mirroring ValidateParserConfig.
func ValidateDocumentParserConfig(parserConfig map[string]interface{}) ([]string, error) {
	dropped := DropUnscopedParserConfigKeys(parserConfig)
	return dropped, validateDatasetParserConfigSize(parserConfig)
}

// NormalizeDatasetID validates the dataset ID format and returns its
// dash-less UUID form. Exported so HTTP handlers can mirror the pydantic
// UUID validation of the Python request models (error code 101).
func NormalizeDatasetID(id string) (string, error) {
	return normalizeDatasetID(id)
}

func normalizeDatasetID(id string) (string, error) {
	parsedUUID, err := uuid.Parse(id)
	if err != nil {
		return "", errors.New("Invalid UUID format")
	}
	if parsedUUID == (uuid.UUID{}) {
		return "", errors.New("Invalid UUID format")
	}
	return strings.ReplaceAll(parsedUUID.String(), "-", ""), nil
}

// datasetLanguageLimit mirrors the max_length of CreateDatasetReq.language in
// the Python request model.
const datasetLanguageLimit = 32

// normalizeDatasetLanguage trims a dataset language and applies the same
// constraints as CreateDatasetReq.language in Python
// (strip_whitespace=True, min_length=1, max_length=32), so both backends accept
// and reject the same values. The length is counted in characters, not bytes,
// because pydantic counts characters — a byte count would reject valid
// non-ASCII language names well below the documented limit.
func normalizeDatasetLanguage(language string) (string, error) {
	normalized := strings.TrimSpace(language)
	if normalized == "" {
		return "", errors.New("String should have at least 1 character")
	}
	if utf8.RuneCountInString(normalized) > datasetLanguageLimit {
		return "", fmt.Errorf("String should have at most %d characters", datasetLanguageLimit)
	}
	return normalized, nil
}

// pythonStringListRepr renders a string slice the way Python prints a list of
// strings, e.g. ['a', 'b'], for error messages that mirror the Python API.
func pythonStringListRepr(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, "'"+item+"'")
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func canvasAccessibleForUser(ctx context.Context, userID, canvasID string) (bool, error) {
	tenantIDs, _ := dao.NewUserTenantDAO().GetTenantIDsByUserID(ctx, dao.DB, userID)
	return dao.NewUserCanvasDAO().Accessible(ctx, dao.DB, canvasID, userID, tenantIDs), nil
}

func parserConfigValueOrEmptyList(parserConfig map[string]interface{}, key string) interface{} {
	if parserConfig == nil {
		return []interface{}{}
	}
	value, ok := parserConfig[key]
	if !ok || value == nil {
		return []interface{}{}
	}
	return value
}

func datasetConnectorsOrEmpty(connectors []*dao.ConnectorDatasetListItem) []*dao.ConnectorDatasetListItem {
	if connectors == nil {
		return make([]*dao.ConnectorDatasetListItem, 0)
	}
	return connectors
}

func datasetUpdateParserID(req service.UpdateDatasetRequest) (string, bool, error) {
	parserID := ""
	provided := false
	if req.ParserID != nil {
		parserID = strings.TrimSpace(*req.ParserID)
		provided = true
	}
	if !provided {
		return "", false, nil
	}
	canonicalID, err := canonicalDatasetParserID(parserID)
	if err != nil {
		return "", true, err
	}
	return canonicalID, true, nil
}

func datasetUpdateEmbeddingID(req service.UpdateDatasetRequest) (string, bool, error) {
	embdID := ""
	provided := false
	if req.EmbdID != nil {
		embdID = strings.TrimSpace(*req.EmbdID)
		provided = true
	}
	if req.EmbeddingModel != nil {
		embdID = strings.TrimSpace(*req.EmbeddingModel)
		provided = true
	}
	if !provided {
		return "", false, nil
	}
	if err := validateDatasetEmbeddingModel(embdID); err != nil {
		return "", true, err
	}
	return embdID, true, nil
}

func preserveDatasetParserConfigState(next, existing entity.JSONMap, incoming map[string]interface{}) entity.JSONMap {
	if next == nil {
		next = entity.JSONMap{}
	}
	var mm map[string]any
	if incoming != nil {
		mm = extractorNodeMetadata(incoming)
	}
	if mm == nil && existing != nil {
		mm = extractorNodeMetadata(existing)
	}
	if mm != nil {
		next["metadata"] = mm
	}
	// Resolve the dataset-level parent_child setting (component-scoped on a
	// chunker node) and scope it onto every chunker node in next. There is no
	// flat aggregation key.
	parentChild := resolveParentChild(incoming, existing)
	if parentChild != nil {
		for componentID, value := range next {
			if !pipelinepkg.IsChunkerComponent(componentID) {
				continue
			}
			params, ok := value.(map[string]interface{})
			if !ok {
				params = map[string]interface{}{}
				next[componentID] = params
			}
			params["parent_child"] = parentChild
		}
	}
	requestedChildren := make(map[string]interface{})
	for componentID, value := range incoming {
		if !pipelinepkg.IsChunkerComponent(componentID) {
			continue
		}
		if requested, ok := value.(map[string]interface{}); ok {
			if enabled, provided := requested["enable_children"].(bool); provided && !enabled {
				requestedChildren[componentID] = []string{}
			} else if _, provided := requested["children_delimiters"]; provided {
				if params, ok := next[componentID].(map[string]interface{}); ok {
					requestedChildren[componentID] = params["children_delimiters"]
				}
			}
		}
	}
	// Re-derive delimiters from parent_child, then keep explicit chunker edits
	// (or an existing chunker setting on a partial update) over that fallback.
	parentChildConfig := map[string]interface{}{}
	for componentID, value := range incoming {
		if pipelinepkg.IsChunkerComponent(componentID) {
			parentChildConfig[componentID] = value
		}
	}
	pipelinepkg.ApplyParentChildChunkerConfig(next, parentChildConfig)
	parentChildUpdated := parentChild != nil
	for componentID, value := range next {
		if !pipelinepkg.IsChunkerComponent(componentID) {
			continue
		}
		params, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		if delimiters, provided := requestedChildren[componentID]; provided {
			params["children_delimiters"] = delimiters
			continue
		}
		if parentChildUpdated {
			continue
		}
		if previous, ok := existing[componentID].(map[string]interface{}); ok {
			if delimiters, present := previous["children_delimiters"]; present {
				params["children_delimiters"] = delimiters
			}
			if enabled, present := previous["enable_children"]; present {
				params["enable_children"] = enabled
			}
		}
	}
	return next
}

// extractorNodeMetadata returns the modular metadata object ({enabled, metadata,
// built_in_metadata}) from the first Extractor node in a parser_config, or nil
// if none is present. Metadata is component-scoped under Extractor nodes; this
// helper reads it back for preservation across partial updates.
func extractorNodeMetadata(parserConfig map[string]interface{}) map[string]any {
	if parserConfig == nil {
		return nil
	}
	for cpnID, raw := range parserConfig {
		lower := strings.ToLower(cpnID)
		if !strings.HasPrefix(lower, "extractor:") && !strings.HasPrefix(lower, "extractor_") {
			continue
		}
		params, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if meta, ok := params["metadata"].(map[string]any); ok {
			return meta
		}
	}
	return nil
}

// resolveParentChild extracts the dataset-level parent_child setting from one or
// more parser_configs. It reads the component-scoped "parent_child" sub-object on
// a chunker node only; flat top-level keys are not accepted. Returns nil when
// absent.
func resolveParentChild(configs ...map[string]interface{}) map[string]any {
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		for componentID, raw := range cfg {
			if !pipelinepkg.IsChunkerComponent(componentID) {
				continue
			}
			params, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if pc, ok := params["parent_child"].(map[string]any); ok {
				return pc
			}
		}
	}
	return nil
}

func parserConfigJSONMap(value interface{}) entity.JSONMap {
	switch typed := value.(type) {
	case nil:
		return nil
	case entity.JSONMap:
		return typed
	case map[string]interface{}:
		return entity.JSONMap(typed)
	default:
		return nil
	}
}

func cloneJSONMap(source entity.JSONMap) entity.JSONMap {
	if source == nil {
		return nil
	}
	cloned := make(entity.JSONMap, len(source))
	for key, value := range source {
		cloned[key] = cloneJSONValue(value)
	}
	return cloned
}

func cloneJSONValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		nested := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			nested[key] = cloneJSONValue(item)
		}
		return nested
	case []interface{}:
		nested := make([]interface{}, len(typed))
		for idx, item := range typed {
			nested[idx] = cloneJSONValue(item)
		}
		return nested
	default:
		return typed
	}
}

func normalizeMetadataConfigFields(fields []service.MetadataConfigField, fieldName string) ([]map[string]interface{}, error) {
	normalizedFields := make([]map[string]interface{}, 0, len(fields))
	for i, field := range fields {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			return nil, fmt.Errorf("%s[%d].key is required", fieldName, i)
		}
		if len(key) > 255 {
			return nil, fmt.Errorf("%s[%d].key should have at most 255 characters", fieldName, i)
		}
		fieldType := strings.TrimSpace(field.Type)
		if _, ok := datasetAllowedMetadataTypes[fieldType]; !ok {
			return nil, fmt.Errorf("%s[%d].type should be one of 'string', 'list', 'time' or 'number'", fieldName, i)
		}
		if field.Description != nil && len(*field.Description) > 65535 {
			return nil, fmt.Errorf("%s[%d].description should have at most 65535 characters", fieldName, i)
		}
		normalizedFields = append(normalizedFields, map[string]interface{}{
			"key":         key,
			"type":        fieldType,
			"description": field.Description,
			"enum":        field.Enum,
		})
	}
	return normalizedFields, nil
}
