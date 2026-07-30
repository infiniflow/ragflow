// Package golden holds the canonical, committed sample corpus and the
// structural-metric helpers used by the knowledge_compiler golden gates
// (PORT_PLAN.md 缺口 C / 缺口 E).
//
// The corpus is intentionally small and stable so that Go-side product counts
// and clustering structure can be LOCKED as a required CI gate. When a live
// Python baseline is available, the same corpus is fed to
// rag/advanced_rag/knowlege_compile to produce the reference numbers recorded
// beside the fixtures in this package.
package golden

import (
	"fmt"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// FixedCorpus is the canonical sample document. It describes a small, coherent
// ecosystem of named entities (Alpha/Beta/Gamma/Delta/Epsilon) with dense
// cross-references so structure extraction, wiki synthesis, and raptor
// clustering all have meaningful signal.
func FixedCorpus() []common.Chunk {
	texts := []string{
		"Alpha is a large language model developed by the Beta research lab.",
		"Beta lab also created Gamma, a retrieval engine for knowledge bases.",
		"Gamma indexes documents and returns the most relevant passages.",
		"Delta is a vector database that stores embeddings produced by Alpha.",
		"Epsilon is an evaluation benchmark comparing retrieval systems.",
		"Alpha and Delta integrate to power semantic search over documents.",
		"Gamma and Epsilon both measure retrieval quality on the same dataset.",
		"Beta lab published a survey covering Alpha, Gamma, and Delta.",
		"The knowledge graph links Alpha, Beta, Gamma, Delta, and Epsilon.",
		"RAG pipelines combine Alpha with Gamma to answer user questions.",
		"Embeddings from Alpha are stored in Delta for fast nearest-neighbor search.",
		"Epsilon scores show Gamma outperforms naive retrieval on Delta indices.",
	}
	chunks := make([]common.Chunk, len(texts))
	for i, t := range texts {
		chunks[i] = common.Chunk{
			ID:   fmt.Sprintf("chunk-%02d", i+1),
			Text: t,
		}
	}
	return chunks
}

// ChunksToAny converts the fixed corpus (or any chunk list) into the runtime
// inputs shape expected by KnowledgeCompilerComponent.Invoke.
func ChunksToAny(chunks []common.Chunk) []any {
	out := make([]any, len(chunks))
	for i, c := range chunks {
		out[i] = map[string]any{"id": c.ID, "text": c.Text}
	}
	return out
}
