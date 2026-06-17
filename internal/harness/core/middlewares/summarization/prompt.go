// Package summarization provides prompt templates.
package summarization

// Action types for EmitInternalEvents
const (
	ActionTypeBeforeSummarize  = "summarize:before"
	ActionTypeAfterSummarize   = "summarize:after"
	ActionTypeGenerateSummary  = "summarize:generate"
)
