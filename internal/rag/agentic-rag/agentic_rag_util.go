package agentic_rag

import (
	"context"
	"log"

	"ragflow/internal/rag/agentic-rag/runtime"
)

// step reports one user-visible reasoning stage of the run: it writes the
// DEVELOPER log line and delivers the same sentence as a step (think block +
// structured event) through the reporter bound on ctx.
//
// Both projections come from this one call, so they cannot drift; and because a
// stage declares its own step, the developer log is free to carry diagnostics
// that must never reach the user — no allowlist of tags, and rewording a log
// line can never hide (or leak) a step.
func step(ctx context.Context, logger *log.Logger, stage, format string, args ...any) {
	runtime.StepsFrom(ctx).Stage(stage, format, args...)
}
