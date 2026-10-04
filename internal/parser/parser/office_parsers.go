package parser

import "errors"

// ErrOfficeCGORequired is returned by the office-family parsers
// (DOC/DOCX/PPT/PPTX) when the build has no native office_oxide backend.
// The native engine (internal/deepdoc/parser/office) reports its own
// ErrOfficeCGORequired sentinel, which the facade wraps into this one so
// callers see a stable, parser-package error regardless of which layer
// reports the missing engine.
var ErrOfficeCGORequired = errors.New("parser: office family requires CGO (office_oxide)")
