package parser

import "context"

// parseSpreadsheetWithTCADP sends binary spreadsheet (XLSX/XLS/CSV) data
// to the TCADP cloud reconstruction service. Thin wrapper over the shared
// parseWithTCADP core — env-fallbacks and request construction live there.
func parseSpreadsheetWithTCADP(ctx context.Context, filename string, data []byte, fileType string, tcadpAPIServer, tcadpAPIKey, tableResultType, markdownImageResponseType string, outputFormat string) ParseResult {
	return parseWithTCADP(ctx, filename, data, fileType,
		tcadpAPIServer, tcadpAPIKey, tableResultType, markdownImageResponseType, outputFormat)
}
