//go:build !ee

package runtime

import "context"

// WithIOProgress is the community-edition stub for enterprise runtime I/O
// progress reporting.
func WithIOProgress(ctx context.Context) (context.Context, func()) {
	return ctx, func() {}
}

// TrackIO is a no-op in the community edition.
func TrackIO(context.Context, string, string, string) func() {
	return func() {}
}

// ReportIOProgress is a no-op in the community edition.
func ReportIOProgress(context.Context, string, string) {}
