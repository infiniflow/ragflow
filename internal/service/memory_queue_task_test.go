package service

import (
	"strings"
	"testing"

	"ragflow/internal/engine"
	natsengine "ragflow/internal/engine/nats"
)

// queueMemoryTask is the agent-canvas memory-save publish path. When the
// message queue engine is a NatsEngine whose Init failed at boot, publishing
// must surface a clean error (caught by the Message component's best-effort
// memory save) rather than panicking and failing the whole canvas run.
func TestQueueMemoryTaskUninitializedQueueReturnsError(t *testing.T) {
	previous := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(natsengine.NewNatsEngine("127.0.0.1", 1))
	t.Cleanup(func() { engine.SetMessageQueueEngine(previous) })

	err := queueMemoryTask(
		t.Context(),
		"task-1",
		"mem-1",
		"tenant-1",
		1,
		MemoryMessage{UserID: "u1", AgentID: "agent-1", SessionID: "s1", UserInput: "hi", AgentResponse: "hello"},
	)
	if err == nil {
		t.Fatal("queueMemoryTask with uninitialized MQ engine: err = nil, want publish error")
	}
	if !strings.Contains(err.Error(), "not properly initialized") {
		t.Fatalf("queueMemoryTask err = %v, want engine-not-initialized error", err)
	}
}
