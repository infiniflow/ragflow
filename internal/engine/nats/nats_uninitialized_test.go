package nats

import (
	"strings"
	"testing"
)

// An engine whose Init failed (or was never called) must return errors from
// its public methods instead of panicking on nil fields. Regression test for
// the nil-deref panic at PublishTask: the server bootstrap only logs message
// queue init failures, so a half-initialized engine stays registered
// process-wide and every later task publish crashed the caller.
func TestUninitializedEngineMethodsReturnErrors(t *testing.T) {
	// Nothing listens on this port; Init() is deliberately not called so the
	// engine keeps its zero-value (nil) connection, jetstream and consumer.
	e := NewNatsEngine("127.0.0.1", 1)

	if err := e.PublishTask("tasks.RAGFLOW", []byte("{}")); err == nil || !strings.Contains(err.Error(), "not properly initialized") {
		t.Fatalf("PublishTask on uninitialized engine: err = %v, want 'not properly initialized'", err)
	}

	if _, err := e.GetMessages(1); err == nil || !strings.Contains(err.Error(), "not properly initialized") {
		t.Fatalf("GetMessages on uninitialized engine: err = %v, want 'not properly initialized'", err)
	}

	if _, err := e.ShowMessageQueue(); err == nil || !strings.Contains(err.Error(), "not properly initialized") {
		t.Fatalf("ShowMessageQueue on uninitialized engine: err = %v, want 'not properly initialized'", err)
	}

	if status := e.CheckStatus(); !strings.Contains(status, "not properly initialized") {
		t.Fatalf("CheckStatus on uninitialized engine = %q, want 'not properly initialized'", status)
	}
}
