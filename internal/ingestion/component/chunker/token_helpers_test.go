package chunker

import (
	"strings"
)

// hardSplitBenchText builds a realistic oversized boundary-less run: a long
// unbroken sequence without sentence delimiters, mixing CJK and ASCII like a
// dense paragraph of a scanned/OCR'd document. This is exactly the input that
// hardSplitPiece must token-split, and the reason the function exists. Sized
// so a single hardSplitPiece call takes on the order of milliseconds: large
// enough for the per-iteration encode work to dominate a benchmark, small
// enough that every test sharing it stays fast. Shared by benchmarks and
// fallback-path tests alike.
func hardSplitBenchText() string {
	var b strings.Builder
	const sentence = "企业知识库检索系统需要高效处理海量非结构化文档的解析切分与向量化入库RAGFlow整合了文档理解文本切分混合检索与重排等多个环节Knowledgebase ingestion splits long passages into token-bounded chunks for embedding. "
	for b.Len() < 12_000 {
		b.WriteString(sentence)
	}
	return b.String()
}
