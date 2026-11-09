//go:build !ee

package task

import "context"

type kcLLMProgressReporter interface {
	begin(ctx context.Context) func()
	report(ctx context.Context, message string)
}

type noopKCLLMProgressReporter struct{}

func newKCLLMProgressReporter() kcLLMProgressReporter {
	return noopKCLLMProgressReporter{}
}

func (noopKCLLMProgressReporter) begin(context.Context) func() {
	return func() {}
}

func (noopKCLLMProgressReporter) report(context.Context, string) {}
