package parser

import "context"

// parsePresentationWithTCADP sends binary presentation (PPTX/PPT) data
// to the TCADP cloud reconstruction service. Thin wrapper over the shared
// parseWithTCADP core — env-fallbacks and request construction live there.
func parsePresentationWithTCADP(ctx context.Context, filename string, data []byte, fileType string,
	tcadpAPIServer, tcadpAPIKey, tableResultType, markdownImageResponseType string,
	outputFormat string,
) ParseResult {
	return parseWithTCADP(ctx, filename, data, fileType,
		tcadpAPIServer, tcadpAPIKey, tableResultType, markdownImageResponseType, outputFormat)
}
