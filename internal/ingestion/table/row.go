package table

import "fmt"

// RowIdentity returns the hash input that identifies a spreadsheet row
// chunk by where it came from, or ok=false for any other chunk. Two different
// source rows that render the same text, and the same row re-parsed under
// different column roles, both keep this identity.
func RowIdentity(ck map[string]any) (string, bool) {
	src, ok := ck["table_row_source"].(map[string]any)
	if !ok {
		return "", false
	}
	sheet, okSheet := jsonNumber(src["sheet_index"])
	row, okRow := jsonNumber(src["source_row"])
	if !okSheet || !okRow || row == 0 {
		return "", false
	}
	return fmt.Sprintf("table-row:v1:%d:%d", sheet, row), true
}

// jsonNumber reads a number that reaches this layer either as an int (in the
// same process) or as a float64 (after a JSON round trip through the Tokenizer
// or a checkpoint).
func jsonNumber(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
