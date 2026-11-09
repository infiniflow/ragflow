//go:build !ee

package task

import "context"

type noopKCLLMProgressReporter struct{}

func newKCLLMProgressReporter() kcLLMProgressReporter {
	return noopKCLLMProgressReporter{}
}

func (noopKCLLMProgressReporter) begin(context.Context) func() {
	return func() {}
}

func (noopKCLLMProgressReporter) report(context.Context, string) {}
