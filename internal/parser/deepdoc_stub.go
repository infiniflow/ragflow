//go:build !cgo

package parser

// NewTableBuilderFor returns a SaasDeepDocTableBuilder by default when cgo is
// not available. Non-cgo builds cannot speak to any real TSR service.
func NewTableBuilderFor(doc DocAnalyzer) TableBuilder {
	return NewSaasDeepDocTableBuilder(doc)
}
