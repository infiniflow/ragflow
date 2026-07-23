package pipeline

import "ragflow/internal/utility"

// NormalizeParserConfigPages walks cfg and normalizes the "pages" field under
// every component's filetype setup. When normalization yields a non-nil
// result, the normalized value is written back; when it yields nil (all
// ranges invalid/empty), the original value is left untouched so the
// persisted config stays consistent with what the frontend submitted.
//
// The walk is generic (not hardcoded to "pdf"): any filetype setup carrying a
// "pages" key is normalized. Setups without "pages" are skipped, so this is
// safe to run on any parser_config and never creates or deletes keys other
// than overwriting "pages" in place.
func NormalizeParserConfigPages(cfg map[string]any) {
	if cfg == nil {
		return
	}
	for _, v := range cfg {
		params, ok := v.(map[string]any)
		if !ok {
			continue
		}
		for _, fv := range params {
			setup, ok := fv.(map[string]any)
			if !ok {
				continue
			}
			raw, ok := setup["pages"]
			if !ok {
				continue
			}
			if normalized := utility.NormalizePDFPages(raw); normalized != nil {
				setup["pages"] = normalized
			}
			// nil (all invalid/empty): leave the original value untouched.
		}
	}
}
