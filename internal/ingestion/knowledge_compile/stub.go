//go:build !ee

package knowledge_compile

import "context"

func withKnowledgeCompileLLMProgress(
	ctx context.Context,
	_ func(context.Context, string, string),
) (context.Context, func()) {
	return ctx, func() {}
}
