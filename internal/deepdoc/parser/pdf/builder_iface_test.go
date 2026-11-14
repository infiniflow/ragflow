package pdf

import (
	"ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// Compile-time guarantees that the pdf-package TableBuilder fakes (and the
// production DeepDocTableBuilder reachable from here) satisfy the
// interface after the FinalizeGrid / AttachSpanInfo extension. Missing a
// method on any implementer fails the build (RED). The cgo&&manual fake
// PythonIntermediateTableBuilder is covered by the same extension in
// replay_analyzer_test.go but is not compiled in the default build.
var (
	_ pdf.TableBuilder = (*staticTableBuilder)(nil)
	_ pdf.TableBuilder = (*table.DeepDocTableBuilder)(nil)
)
