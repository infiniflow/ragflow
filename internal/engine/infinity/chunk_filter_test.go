package infinity

import (
	"testing"

	"ragflow/internal/engine/types"
)

func TestShouldDefaultAvailableFilter(t *testing.T) {
	if !shouldDefaultAvailableFilter(&types.SearchRequest{}, false) {
		t.Fatal("ordinary retrieval must default to available_int=1")
	}
	if shouldDefaultAvailableFilter(&types.SearchRequest{IncludeUnavailable: true}, false) {
		t.Fatal("management list must not add the retrieval availability default")
	}
	if shouldDefaultAvailableFilter(&types.SearchRequest{}, true) {
		t.Fatal("skill indexes do not use chunk availability")
	}
}
