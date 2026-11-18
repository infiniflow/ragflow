package parser

// pdfOutlinesFromPDF reads a PDF's bookmark tree. The native engine backs it
// under cgo; without cgo it reports no bookmarks.
var pdfOutlinesFromPDF = pdfOutlinesFromData

// AttachPDFOutlines populates File["outline"] from the source PDF's bookmarks,
// mutating result.File in place. Remote engines return document content only,
// so their results carry an empty outline and the title chunker, which derives
// heading levels from bookmarks, has nothing to work with. A result that
// already carries an outline keeps it, so the engine that read the bookmarks
// stays authoritative. Both boundaries that hand out a PDF parse result — the
// built-in parse methods and the ingestion dispatch — hold the source bytes and
// call this themselves.
func AttachPDFOutlines(result ParseResult, data []byte) ParseResult {
	if result.Err != nil || result.File == nil {
		return result
	}
	if existing, ok := result.File["outline"].([]map[string]any); ok && len(existing) > 0 {
		return result
	}
	if outlines := pdfOutlinesFromPDF(data); len(outlines) > 0 {
		result.File["outline"] = outlinesToFileMeta(outlines)
	}
	return result
}
