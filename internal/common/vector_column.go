package common

import (
	"fmt"
	"strconv"
	"strings"
)

// VectorColumnName returns the storage column name for a dense vector of the
// given dimension. All search backends (elasticsearch, infinity, opensearch,
// vastbase, oceanbase, serenedb) and the indexing/query code use this single
// convention, whose format is "q_<dim>_vec".
func VectorColumnName(dim int) string {
	return fmt.Sprintf("q_%d_vec", dim)
}

// IsVectorColumn reports whether key names a dense-vector column. A key matches
// when it has the "q_" prefix and the "_vec" suffix — equivalent to
// HasPrefix(key, "q_") && HasSuffix(key, "_vec"). Engine-internal shadow columns
// such as "q_<dim>_vec_n" (SereneDB normalization) are therefore correctly
// excluded. Use VectorColumnDim to recover the dimension from a matching key.
func IsVectorColumn(key string) bool {
	return strings.HasPrefix(key, "q_") && strings.HasSuffix(key, "_vec")
}

// VectorColumnDim extracts the dimension from a vector-column name. It returns
// ok=false when key is not a vector column (IsVectorColumn is false) or its
// embedded dimension is not a valid non-negative integer.
func VectorColumnDim(key string) (int, bool) {
	if !IsVectorColumn(key) {
		return 0, false
	}
	const prefix = "q_"
	const suffix = "_vec"
	// Guard the empty-middle edge ("q_vec" passes IsVectorColumn but has no
	// dimension to parse); without this the substring below would panic.
	if len(key) < len(prefix)+len(suffix) {
		return 0, false
	}
	n, err := strconv.Atoi(key[len(prefix) : len(key)-len(suffix)])
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
