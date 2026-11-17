package migrationguard

import (
	"sort"
	"testing"
)

func TestScanReal(t *testing.T) {
	dirs := []string{
		"../../compilation/extractor",
		"../../component",
		"../../component/chunker",
		"../../component/globals",
		"../../component/knowledge_compiler/common",
		"../../component/knowledge_compiler/golden",
		"../../component/knowledge_compiler",
		"../../component/knowledge_compiler/mindmap",
		"../../component/knowledge_compiler/structure",
		"../../component/knowledge_compiler/tree",
		"../../component/knowledge_compiler/wiki",
		"../../component/schema",
		"../../knowledge_compile",
		"../../pipeline",
		"../../service",
		"../../task/indexdoc",
		"../../task",
	}
	for _, d := range dirs {
		hits, err := scan(d)
		if err != nil {
			t.Logf("ERR %s: %v", d, err)
			continue
		}
		sort.Strings(hits)
		for _, h := range hits {
			t.Logf("%s :: %s", d, h)
		}
		t.Logf("--- %s: %d hits", d, len(hits))
	}
}
