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
