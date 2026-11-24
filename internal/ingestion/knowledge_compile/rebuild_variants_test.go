package knowledge_compile

import (
	"context"
	"reflect"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// fakeReader is a Reader double that returns a canned product set for the
// recoverDocVariantsBatch tests.
type fakeReader struct {
	products []kccommon.Product
	// batchSizes records the doc_id slice length passed to each
	// LoadDocProductsByDocs call, so chunking tests can assert bounded batches.
	batchSizes []int
}

func (f *fakeReader) LoadDocProducts(context.Context, string, string, string) ([]kccommon.Product, error) {
	return f.products, nil
}

func (f *fakeReader) LoadDocProductsByDocs(_ context.Context, _, _ string, docIDs []string) (map[string][]kccommon.Product, error) {
	f.batchSizes = append(f.batchSizes, len(docIDs))
	out := make(map[string][]kccommon.Product, len(docIDs))
	for _, docID := range docIDs {
		out[docID] = nil
	}
	for _, p := range f.products {
		out[p.DocID] = append(out[p.DocID], p)
	}
	return out, nil
}

func (f *fakeReader) SearchSimilar(context.Context, string, string, kccommon.Variant, []float64, int, float64) (kccommon.Product, float64, error) {
	return kccommon.Product{}, 0, nil
}

// TestRecoverDocTypesFromFallbackVariant covers B1a: a product without an
// authoritative kind falls back to its reverse-mapped variant. This is the
// branch the deleted recoverDocTypes wrapper used to exercise; it now lives on
// the shared recoverDocTypesFrom helper used by the rebuild batch path.
func TestRecoverDocTypesFromFallbackVariant(t *testing.T) {
	got, taskTypes, err := recoverDocTypesFrom([]kccommon.Product{
		{DocID: "d1", Variant: kccommon.VariantWiki},
	})
	if err != nil {
		t.Fatalf("recoverDocTypesFrom error: %v", err)
	}
	if len(got) != 1 || got[0] != string(kccommon.VariantWiki) {
		t.Fatalf("recoverDocTypesFrom fallback = %v, want [wiki]", got)
	}
	if !reflect.DeepEqual(taskTypes, []string{kccommon.TaskTypeWiki}) {
		t.Fatalf("recoverDocTypesFrom fallback task types = %v, want [Wiki]", taskTypes)
	}
}
