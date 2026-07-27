// Package canvas — pregel engine init for tests.
//
// This file ensures the Pregel execution engine is registered (via its
// init()) when any canvas test creates a CompiledGraph via Compile and
// calls Graph.Invoke. Without it, compiledGraph.run() returns
// "graph: pregel engine not installed".
package canvas

import _ "ragflow/internal/harness/graph/pregel"
