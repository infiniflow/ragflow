package knowledge_compile

import (
	"context"
	"reflect"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// fakeReader is a Reader double that returns a canned product set for
// recoverDocTypes tests.
type fakeReader struct {
	products []kccommon.Product
}

func (f *fakeReader) LoadDocProducts(context.Context, string, string, string) ([]kccommon.Product, error) {
	return f.products, nil
}

func (f *fakeReader) SearchSimilar(context.Context, string, string, kccommon.Variant, []float64, int, float64) (kccommon.Product, float64, error) {
	return kccommon.Product{}, 0, nil
}

// TestRecoverDocTypes_AuthoritativeKind covers B1a/O2a: the authoritative
// Product.Kind (compilation_template_kind_kwd) is mapped through KindToVariant;
// results are sorted/deduped.
func TestRecoverDocVariants_AuthoritativeKind(t *testing.T) {
	c := &Consumer{reader: &fakeReader{products: []kccommon.Product{
		{DocID: "d1", Kind: "structure"},
		{DocID: "d1", Kind: "tree"},
		// duplicate variant, deduped
		{DocID: "d1", Kind: "structure"},
	}}}
	got, taskTypes, err := c.recoverDocTypes(context.Background(), "t1", "kb1", "d1")
	if err != nil {
		t.Fatalf("recoverDocTypes error: %v", err)
	}
	want := []string{string(kccommon.VariantStructure), string(kccommon.VariantTree)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recoverDocTypes variants = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(taskTypes, []string{kccommon.TaskTypeGraph, kccommon.TaskTypeTree}) {
		t.Fatalf("recoverDocTypes task types = %v, want [Graph Tree]", taskTypes)
	}
}

// TestRecoverDocTypes_UnknownKindHardFails covers O2a: a whitelist-out
// authoritative kind aborts recovery (returns an error) so the rebuild does not
// proceed with an incomplete variant set.
func TestRecoverDocVariants_UnknownKindHardFails(t *testing.T) {
	c := &Consumer{reader: &fakeReader{products: []kccommon.Product{
		{DocID: "d1", Kind: "structure"},
		{DocID: "d1", Kind: "garbage"},
	}}}
	if _, _, err := c.recoverDocTypes(context.Background(), "t1", "kb1", "d1"); err == nil {
		t.Fatal("recoverDocTypes must hard-fail on an unknown authoritative kind (O2a)")
	}
}

// TestRecoverDocTypes_FallbackVariant covers B1a: a product without an
// authoritative kind falls back to its reverse-mapped variant.
func TestRecoverDocVariants_FallbackVariant(t *testing.T) {
	c := &Consumer{reader: &fakeReader{products: []kccommon.Product{
		{DocID: "d1", Variant: kccommon.VariantWiki},
	}}}
	got, taskTypes, err := c.recoverDocTypes(context.Background(), "t1", "kb1", "d1")
	if err != nil {
		t.Fatalf("recoverDocTypes error: %v", err)
	}
	if len(got) != 1 || got[0] != string(kccommon.VariantWiki) {
		t.Fatalf("recoverDocTypes fallback = %v, want [wiki]", got)
	}
	if !reflect.DeepEqual(taskTypes, []string{kccommon.TaskTypeWiki}) {
		t.Fatalf("recoverDocTypes fallback task types = %v, want [Wiki]", taskTypes)
	}
}
