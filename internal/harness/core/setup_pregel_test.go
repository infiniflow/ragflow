// Package core_test setup: import pregel to trigger pregel.init() which sets
// graph.PregelRunFunc, ensuring compiled.Invoke() uses the Pregel execution
// engine instead of the (now removed) sequential inline fallback.
package core

import _ "ragflow/internal/harness/graph/pregel"
