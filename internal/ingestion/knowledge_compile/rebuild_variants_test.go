package knowledge_compile

import (
	"context"
	"reflect"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// fakeReader is a Reader double that returns a canned product set for
// recoverDocVariants tests.
type fakeReader struct {
	products []kccommon.Product
}

func (f *fakeReader) LoadDocProducts(context.Context, string, string, string) ([]kccommon.Product, error) {
	return f.products, nil
}

func (f *fakeReader) SearchSimilar(context.Context, string, string, kccommon.Variant, []float64, int, float64) (kccommon.Product, float64, error) {
	return kccommon.Product{}, 0, nil
}

// TestRecoverDocVariants_AuthoritativeKind covers B1a/O2a: the authoritative
// Product.Kind (compilation_template_kind_kwd) is mapped through KindToVariant;
// results are sorted/deduped.
func TestRecoverDocVariants_AuthoritativeKind(t *testing.T) {
	c := &Consumer{reader: &fakeReader{products: []kccommon.Product{
		{DocID: "d1", Kind: "structure"},
		{DocID: "d1", Kind: "tree"},
		// duplicate variant, deduped
		{DocID: "d1", Kind: "structure"},
	}}}
	got, err := c.recoverDocVariants(context.Background(), "t1", "kb1", "d1")
	if err != nil {
		t.Fatalf("recoverDocVariants error: %v", err)
	}
	want := []string{string(kccommon.VariantStructure), string(kccommon.VariantTree)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recoverDocVariants = %v, want %v", got, want)
	}
}

// TestRecoverDocVariants_UnknownKindHardFails covers O2a: a whitelist-out
// authoritative kind aborts recovery (returns an error) so the rebuild does not
// proceed with an incomplete variant set.
func TestRecoverDocVariants_UnknownKindHardFails(t *testing.T) {
	c := &Consumer{reader: &fakeReader{products: []kccommon.Product{
		{DocID: "d1", Kind: "structure"},
		{DocID: "d1", Kind: "garbage"},
	}}}
	if _, err := c.recoverDocVariants(context.Background(), "t1", "kb1", "d1"); err == nil {
		t.Fatal("recoverDocVariants must hard-fail on an unknown authoritative kind (O2a)")
	}
}

// TestRecoverDocVariants_FallbackVariant covers B1a: a product without an
// authoritative kind falls back to its reverse-mapped variant.
func TestRecoverDocVariants_FallbackVariant(t *testing.T) {
	c := &Consumer{reader: &fakeReader{products: []kccommon.Product{
		{DocID: "d1", Variant: kccommon.VariantWiki},
	}}}
	got, err := c.recoverDocVariants(context.Background(), "t1", "kb1", "d1")
	if err != nil {
		t.Fatalf("recoverDocVariants error: %v", err)
	}
	if len(got) != 1 || got[0] != string(kccommon.VariantWiki) {
		t.Fatalf("recoverDocVariants fallback = %v, want [wiki]", got)
	}
}
