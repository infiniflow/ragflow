package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/dao"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/service"

	"github.com/google/uuid"
)

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
	graphRaptorQueueDocID    = "graph_raptor_x"
	maximumTaskPageNumber    = int64(100000000)
	serverQueueNamePrefix    = "te"
	defaultEmbeddingCheckNum = 5

	graphPhaseResolutionDone = "resolution_done"
	graphPhaseCommunityDone  = "community_done"
)

// validateParserID validates parser_id against the built-in pipeline registry.
func validateParserID(chunkMethod string) error {
	if chunkMethod == "knowledge_graph" {
		return nil
	}
	registry, err := pipelinepkg.DefaultRegistry()
	if err != nil || registry == nil {
		return errors.New("parser_id validation unavailable: builtin pipeline registry not loaded")
	}
	if registry.IsValid(chunkMethod) {
		return nil
	}
	return parserIDError()
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
		return fmt.Errorf("Input should be '%s'", refs[0])
	default:
		return fmt.Errorf("Input should be %s or '%s'", quoteList(refs[:len(refs)-1]), refs[len(refs)-1])
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
		return errors.New("Missing MIME prefix. Expected format: data:<mime>;base64,<data>")
	}
	prefix, _, _ := strings.Cut(avatar, ",")
	if !strings.HasPrefix(prefix, "data:") {
		return errors.New("Invalid MIME prefix format. Must start with 'data:'")
	}
	mimeType, _, _ := strings.Cut(strings.TrimPrefix(prefix, "data:"), ";")
	if _, ok := datasetSupportedAvatarMIMETypes[mimeType]; !ok {
		return errors.New("Unsupported MIME type. Allowed: [image/jpeg image/png]")
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
		return errors.New("Embedding model identifier must follow <model_name>@<provider> format")
	}

	parts := strings.SplitN(embeddingModel, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("Both model_name and provider must be non-empty strings")
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return errors.New("Both model_name and provider must be non-empty strings")
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

func canvasAccessibleForUser(userID, canvasID string) (bool, error) {
	tenantIDs, _ := dao.NewUserTenantDAO().GetTenantIDsByUserID(userID)
	return dao.NewUserCanvasDAO().Accessible(canvasID, userID, tenantIDs), nil
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
	if err := validateParserID(parserID); err != nil {
		return "", true, err
	}
	return parserID, true, nil
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

func normalizeDatasetUpdateExt(ext map[string]interface{}) map[string]interface{} {
	if ext == nil {
		return nil
	}
	updates := make(map[string]interface{}, len(ext))
	for key, value := range ext {
		switch key {
		case "chunk_method":
			updates["parser_id"] = value
		case "token_num", "chunk_num", "parser_config":
			continue
		case "pagerank":
			if v, ok := value.(float64); ok {
				updates[key] = int64(v)
			}
		default:
			updates[key] = value
		}
	}
	return updates
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
