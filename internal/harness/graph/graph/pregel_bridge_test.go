package graph

// This import links the pregel engine into the graph/graph test binary.
// Without it, compiledGraph.Invoke() → "graph: pregel engine not installed"
// because PregelRunFunc is never registered in this isolated test binary.
// The pregel package's init() calls types.SetPregelRunFunc(runCompiledGraph),
// which is the function called by all compiled.Invoke() calls.
//
// This is the standard Go pattern for side-effect imports in tests.
// See also: stdlib database/sql tests import "database/sql/driver" drivers.
import _ "ragflow/internal/harness/graph/pregel"
