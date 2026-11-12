//go:build !ee

package knowledge_compile

import "context"

func withKnowledgeCompileLLMProgress(
	ctx context.Context,
	_, _ string,
) (context.Context, func()) {
	return ctx, func() {}
}

// SubmitCompilerLLMJobs preserves pool submission without enterprise progress reporting.
func SubmitCompilerLLMJobs(ctx context.Context, jobs []CompilerJob) error {
	return SubmitCompilerJobs(ctx, jobs)
}
