package nlp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This is an output-invariant refactor. The old inline spellings
// `"q_" + fmt.Sprintf("%d", n) + "_vec"` / `"q_"+fmt.Sprintf("%d", n)+"_vec"`
// and `!HasPrefix("q_") || !HasSuffix("_vec")` produce bytes identical to the
// shared `common.VectorColumnName` / `common.IsVectorColumn` helpers, so a
// behavioral test cannot distinguish them. The migration's only real risk is
// leaving a write or scan site behind on the old spelling, so we assert
// structurally that no inline duplication of the q_<dim>_vec vocabulary
// remains in datasetnav.go.
func TestDatasetNavVectorColumnNoInlineDuplication(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "datasetnav.go"))
	if err != nil {
		t.Fatalf("read datasetnav.go: %v", err)
	}
	s := string(src)
	for _, bad := range []string{
		`"q_" + fmt.Sprintf`,
		`"q_"+fmt.Sprintf`,
		`"q_" + strconv`,
		`"q_"+strconv`,
		`!strings.HasPrefix(k, "q_") || !strings.HasSuffix(k, "_vec")`,
		`HasPrefix(k, "q_") && HasSuffix(k, "_vec")`,
	} {
		if strings.Contains(s, bad) {
			t.Errorf("inline vector-column vocabulary still present in datasetnav.go: %q", bad)
		}
	}
}
