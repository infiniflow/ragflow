//go:build !ee

package chunker

import "context"

func previewChunks(_ context.Context, chunks []map[string]any) []map[string]any { return chunks }
func skipPreviewMedia(string, []map[string]any) bool                            { return false }
