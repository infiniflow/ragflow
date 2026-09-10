package service

import (
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	natsengine "ragflow/internal/engine/nats"
)

// TestPublishMemoryTaskWakeupContainsOnlyDurableIdentity verifies NATS carries
// no execution input that can diverge from the database record.
func TestPublishMemoryTaskWakeupContainsOnlyDurableIdentity(t *testing.T) {
	publisher := &recordingTaskPublisher{}
	if err := publishMemoryTaskWakeup(publisher, "task-1"); err != nil {
		t.Fatalf("publishMemoryTaskWakeup: %v", err)
	}
	if publisher.subject != common.TaskSubject || len(publisher.messages) != 1 {
		t.Fatalf("published subject/messages = %q/%d", publisher.subject, len(publisher.messages))
	}
	message := publisher.messages[0]
	if message.TaskID != "task-1" || message.TaskType != common.TaskTypeMemory {
		t.Fatalf("published memory wake-up = %+v", message)
	}
}

// publishMemoryTaskWakeup is the agent-canvas memory-save publish path. When the
// message queue engine is a NatsEngine whose Init failed at boot, publishing
// must surface a clean error (caught by the Message component's best-effort
// memory save) rather than panicking and failing the whole canvas run.
func TestQueueMemoryTaskUninitializedQueueReturnsError(t *testing.T) {
	previous := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(natsengine.NewNatsEngine("127.0.0.1", 1))
	t.Cleanup(func() { engine.SetMessageQueueEngine(previous) })

	err := publishMemoryTaskWakeup(NewMessageQueueTaskPublisher(), "task-1")
	if err == nil {
		t.Fatal("publishMemoryTaskWakeup with uninitialized MQ engine: err = nil, want publish error")
	}
	if !strings.Contains(err.Error(), "not properly initialized") {
		t.Fatalf("publishMemoryTaskWakeup err = %v, want engine-not-initialized error", err)
	}
}
