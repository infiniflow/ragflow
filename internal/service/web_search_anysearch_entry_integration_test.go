//go:build integration

package service

import (
	"os"
	"testing"
)

// Real AnySearch is called through normal saved Chat configuration and AsyncChat.
// The model is scripted and the SQLite KB is unindexed; this is not a full
// ingest/index/retrieve test or actual model inference.
func TestAnySearchChatEntryPointLive(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH=1 to call the real AnySearch service")
	}
	runAnySearchChatEntryPointModes(t, true)
}
