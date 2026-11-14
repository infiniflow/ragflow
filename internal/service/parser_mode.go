package service

import (
	"errors"
	"fmt"
	"strings"
)

// ParseMode identifies which parser backend a dataset or document uses.
type ParseMode int

const (
	// ParseModeBuiltIn selects a builtin parser template (parser_id).
	ParseModeBuiltIn ParseMode = 1
	// ParseModePipeline selects a user canvas pipeline (pipeline_id).
	ParseModePipeline ParseMode = 2
)

// ParserSelection is the canonical, unambiguous choice of parser backend.
// Exactly one of (Mode == BuiltIn with a non-empty ParserID) or
// (Mode == Pipeline with a non-empty PipelineID) holds. Construct it only via
// FromRequest (from a request) or CurrentSelection (from persisted columns);
// never build it directly. Centralizing construction here removes the old
// three-field ambiguity (parse_type / parser_id / pipeline_id) and the nil
// branch that inferred the mode from whichever ID happened to be present.
type ParserSelection struct {
	Mode       ParseMode
	ParserID   string
	PipelineID string
}

// IsBuiltIn reports whether the selection targets a builtin parser.
func (s *ParserSelection) IsBuiltIn() bool {
	return s != nil && s.Mode == ParseModeBuiltIn
}

// IsPipeline reports whether the selection targets a canvas pipeline.
func (s *ParserSelection) IsPipeline() bool {
	return s != nil && s.Mode == ParseModePipeline
}

// Effective returns the (isPipeline, parserID, pipelineID) representation used
// for storage and DSL loading. Exactly one of parserID/pipelineID is non-empty:
// for BuiltIn the pipeline_id is nil, for Pipeline the parser_id is the stored
// builtin id (ignored by the canvas DSL loader) and pipeline_id carries the
// canvas id.
func (s *ParserSelection) Effective() (isPipeline bool, parserID string, pipelineID *string) {
	if s == nil {
		return false, "", nil
	}
	if s.IsPipeline() {
		pid := s.PipelineID
		return true, s.ParserID, &pid
	}
	return false, s.ParserID, nil
}

func isBlankID(p *string) bool {
	return p == nil || strings.TrimSpace(*p) == ""
}

// FromRequest validates the raw request fields and returns the corresponding
// ParserSelection. It returns (nil, nil) when all three fields are absent —
// the PATCH signal that the whole selection block is omitted and the current
// selection must be kept. Any other partial presence is rejected so a malformed
// request can never silently pick the wrong mode:
//   - no parse_type while an id is present  -> "parse_type is required"
//   - an id contradictory to parse_type      -> explicit mismatch error
//   - an unknown parse_type value            -> "invalid parse_type"
func FromRequest(parseType *int, parserID, pipelineID *string) (*ParserSelection, error) {
	if parseType == nil && isBlankID(parserID) && isBlankID(pipelineID) {
		return nil, nil
	}
	if parseType == nil {
		return nil, errors.New("parse_type is required")
	}
	switch *parseType {
	case 1: // BuiltIn
		if !isBlankID(pipelineID) {
			return nil, errors.New("pipeline_id must not be set when parse_type is BuiltIn")
		}
		if isBlankID(parserID) {
			return nil, errors.New("parser_id is required when parse_type is BuiltIn")
		}
		return &ParserSelection{Mode: ParseModeBuiltIn, ParserID: strings.TrimSpace(*parserID)}, nil
	case 2: // Pipeline
		if !isBlankID(parserID) {
			return nil, errors.New("parser_id must not be set when parse_type is Pipeline")
		}
		if isBlankID(pipelineID) {
			return nil, errors.New("pipeline_id is required when parse_type is Pipeline")
		}
		return &ParserSelection{Mode: ParseModePipeline, PipelineID: strings.TrimSpace(*pipelineID)}, nil
	default:
		return nil, fmt.Errorf("invalid parse_type: %d (must be 1 or 2)", *parseType)
	}
}

// CurrentSelection builds the ParserSelection currently persisted on a
// dataset/document from its parser_id / pipeline_id columns.
func CurrentSelection(parserID string, pipelineID *string) *ParserSelection {
	if pipelineID != nil && strings.TrimSpace(*pipelineID) != "" {
		return &ParserSelection{Mode: ParseModePipeline, ParserID: parserID, PipelineID: strings.TrimSpace(*pipelineID)}
	}
	return &ParserSelection{Mode: ParseModeBuiltIn, ParserID: parserID}
}

// Resolve combines the current selection with a request selection.
//   - Creation (current == nil): the request selection is mandatory, so a nil
//     sel is an error — a dataset/document must explicitly pick a mode.
//   - Update (current != nil): a nil sel means the whole selection block was
//     omitted and the current selection is kept (PATCH semantics); a non-nil
//     sel overrides it (mode switch or same-mode id change).
func Resolve(current, sel *ParserSelection) (*ParserSelection, error) {
	if sel == nil {
		if current == nil {
			return nil, errors.New("parse_type is required")
		}
		return current, nil
	}
	return sel, nil
}
