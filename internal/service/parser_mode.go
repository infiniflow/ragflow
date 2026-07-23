package service

import (
	"errors"
	"fmt"
	"strings"
)

// ValidateParseTypeMode validates parse_type and ensures the corresponding
// identifier field is present. Returns (isBuiltin, isPipeline, error).
//   - parse_type must be 1 (BuiltIn) or 2 (Pipeline); nil is an error.
//   - BuiltIn mode requires parserID to be present and non-empty.
//   - Pipeline mode requires pipelineID to be present and non-empty.
func ValidateParseTypeMode(parseType *int, parserID, pipelineID *string) (isBuiltin bool, isPipeline bool, err error) {
	if parseType == nil {
		return false, false, errors.New("parse_type is required")
	}
	switch *parseType {
	case 1:
		if parserID == nil || strings.TrimSpace(*parserID) == "" {
			return false, false, errors.New("parser_id is required when parse_type is BuiltIn")
		}
		return true, false, nil
	case 2:
		if pipelineID == nil || strings.TrimSpace(*pipelineID) == "" {
			return false, false, errors.New("pipeline_id is required when parse_type is Pipeline")
		}
		return false, true, nil
	default:
		return false, false, fmt.Errorf("invalid parse_type: %d (must be 1 or 2)", *parseType)
	}
}

// ParseModeState carries the persisted IDs that ResolveParseMode falls back to
// when the request does not switch modes.
type ParseModeState struct {
	ParserID   string
	PipelineID *string
}

// ResolveParseMode returns the effective (isPipeline, parserID, pipelineID)
// after applying an update. It is the single source of truth for "which DSL
// should parser_config be cleaned against" and "which mode should reparse
// target", shared by the dataset and document update paths.
//
// parseType is authoritative when present:
//   - 1 (BuiltIn): effParserID = reqParserID (or current); effPipelineID = nil
//     so any prior canvas is cleared. reqPipelineID is ignored.
//   - 2 (Pipeline): effPipelineID = reqPipelineID (or current); effParserID =
//     current.ParserID (parser_id is not applicable in pipeline mode).
//     reqParserID is ignored.
//
// When parseType is nil (no mode switch, e.g. only parser_config changed),
// inherit current mode and apply any incremental ID updates from the request;
// isPipeline is derived from whether effPipelineID is non-empty.
func ResolveParseMode(parseType *int, reqParserID, reqPipelineID *string,
	current ParseModeState) (isPipeline bool, effParserID string, effPipelineID *string) {
	if parseType != nil {
		switch *parseType {
		case 1: // BuiltIn
			effParserID = current.ParserID
			if reqParserID != nil {
				if p := strings.TrimSpace(*reqParserID); p != "" {
					effParserID = p
				}
			}
			return false, effParserID, nil
		case 2: // Pipeline
			effPipelineID = current.PipelineID
			if reqPipelineID != nil {
				effPipelineID = reqPipelineID
			}
			return true, current.ParserID, effPipelineID
		}
	}
	// No mode switch — inherit current state, apply incremental ID updates.
	effParserID = current.ParserID
	if reqParserID != nil {
		if p := strings.TrimSpace(*reqParserID); p != "" {
			effParserID = p
		}
	}
	effPipelineID = current.PipelineID
	if reqPipelineID != nil {
		effPipelineID = reqPipelineID
	}
	isPipeline = effPipelineID != nil && strings.TrimSpace(*effPipelineID) != ""
	return isPipeline, effParserID, effPipelineID
}
