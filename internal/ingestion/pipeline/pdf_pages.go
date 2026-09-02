package pipeline

import "ragflow/internal/utility"

// NormalizeParserConfigPages walks cfg and normalizes the "pages" field under
// every component's filetype setup under fail-fast semantics.
//
// When normalization succeeds, the normalized value is written back. When any
// "pages" value is invalid (non-list, or contains an invalid range), an error
// is returned immediately and the request should be rejected — no partial
// dropping, no保留 of malformed values.
//
// The walk is generic (not hardcoded to "pdf"): any filetype setup carrying a
// "pages" key is normalized. Setups without "pages" are skipped, so this is
// safe to run on any parser_config and never creates or deletes keys other
// than overwriting "pages" in place. Structural mismatches (non-map cpnID
// value, non-map setup) are silently skipped — they are not "pages" errors.
func NormalizeParserConfigPages(cfg map[string]any) error {
	if cfg == nil {
		return nil
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
			normalized, err := utility.NormalizePDFPages(raw)
			if err != nil {
				return err
			}
			if normalized != nil {
				setup["pages"] = normalized
			}
			// nil (no value): leave the key as-is. Empty/null pages are
			// equivalent to "parse all pages"; overwriting with nil would
			// not change behavior but would mutate the map unnecessarily.
		}
	}
	return nil
}
