//go:build manual

package pdf

import (
	"testing"

	"ragflow/internal/common"
)

// restoreLoggerGlobals snapshots the process-wide logger state and reinstalls
// it when t completes. common.InitLogger overwrites common.Logger, common.Sugar
// and the shared log level, so a test that calls it would otherwise leak that
// state into later tests in the same manual-tier binary.
func restoreLoggerGlobals(t *testing.T) {
	t.Helper()
	prevLogger, prevSugar := common.Logger, common.Sugar
	var prevLevel string
	if common.Logger != nil {
		prevLevel = common.GetLogLevel()
	}
	t.Cleanup(func() {
		common.Logger = prevLogger
		common.Sugar = prevSugar
		if prevLogger != nil {
			_ = common.SetLogLevel(prevLevel)
		}
	})
}
