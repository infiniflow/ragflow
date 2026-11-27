//go:build !ee

package task

import (
	"context"
	"time"
)

type pipelinePreviewResult struct{}

func (*TaskContext) IsDebug() bool { return false }
func (*PipelineExecutor) previewOutput(context.Context, map[string]any, time.Time) (*PipelineResult, bool) {
	return nil, false
}
func (*PipelineExecutor) previewTaskID() string { return "" }
func (*PipelineExecutor) preparePreviewInputs(_ string, _ map[string]any, params map[string]any) map[string]any {
	return params
}
func (*PipelineExecutor) publishPreviewResult(string, map[string]any) {}

func (*PipelineExecutor) configurePreview() {}
