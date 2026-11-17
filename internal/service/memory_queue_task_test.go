package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	natsengine "ragflow/internal/engine/nats"
)

// defaultResolver is a fixed resolver used by tests that only care that a wake-up
// is published, isolating them from the global billing service state.
var defaultResolver tierResolver = func(context.Context, string) (common.Tier, error) {
	return common.TierDefault, nil
}

// TestPublishMemoryTaskWakeupRoutesToDefaultLane verifies the wake-up subject is
// the single community queue. A resolved tenant and a resolver error both land on
// the default subject (the single community queue in OSS) instead of silently
// dropping the wake-up.
func TestPublishMemoryTaskWakeupRoutesToDefaultLane(t *testing.T) {
	cases := []struct {
		name     string
		resolver tierResolver
		want     string
	}{
		{"resolved tier", defaultResolver, common.TaskSubject},
		{"resolver error -> default fallback", func(context.Context, string) (common.Tier, error) { return "", errors.New("billing down") }, common.DefaultTierSubject()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			publisher := &recordingTaskPublisher{}
			if err := publishMemoryTaskWakeup(t.Context(), publisher, c.resolver, "tenant-x", "task-1"); err != nil {
				t.Fatalf("publishMemoryTaskWakeup: %v", err)
			}
			if publisher.subject != c.want {
				t.Fatalf("subject = %q, want %q", publisher.subject, c.want)
			}
			if len(publisher.messages) != 1 || publisher.messages[0].TaskID != "task-1" || publisher.messages[0].TaskType != common.TaskTypeMemory {
				t.Fatalf("published = %+v, want one memory wake-up for task-1", publisher.messages)
			}
		})
	}
}

// TestPublishMemoryTaskWakeupContainsOnlyDurableIdentity verifies NATS carries
// no execution input that can diverge from the database record.
func TestPublishMemoryTaskWakeupContainsOnlyDurableIdentity(t *testing.T) {
	publisher := &recordingTaskPublisher{}
	if err := publishMemoryTaskWakeup(t.Context(), publisher, defaultResolver, "tenant-1", "task-1"); err != nil {
		t.Fatalf("publishMemoryTaskWakeup: %v", err)
	}
	// The default resolver selects the default lane, so the wake-up is published
	// to the single community subject.
	if publisher.subject != common.TaskSubject || len(publisher.messages) != 1 {
		t.Fatalf("published subject/messages = %q/%d", publisher.subject, len(publisher.messages))
	}
	message := publisher.messages[0]
	if message.TaskID != "task-1" || message.TaskType != common.TaskTypeMemory {
		t.Fatalf("published memory wake-up = %+v", message)
	}
}

// TestQueueMemoryTaskUninitializedQueueReturnsError verifies that publishMemoryTaskWakeup,
// the agent-canvas memory-save publish path, surfaces a clean error. When the
// queue engine is a NatsEngine whose Init failed at boot (caught by the Message
// component's best-effort memory save) rather than panicking and failing the whole
// canvas run.
func TestQueueMemoryTaskUninitializedQueueReturnsError(t *testing.T) {
	previous := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(natsengine.NewNatsEngine("127.0.0.1", 1))
	t.Cleanup(func() { engine.SetMessageQueueEngine(previous) })

	err := publishMemoryTaskWakeup(t.Context(), NewMessageQueueTaskPublisher(), nil, "tenant-1", "task-1")
	if err == nil {
		t.Fatal("publishMemoryTaskWakeup with uninitialized MQ engine: err = nil, want publish error")
	}
	if !strings.Contains(err.Error(), "not properly initialized") {
		t.Fatalf("publishMemoryTaskWakeup err = %v, want engine-not-initialized error", err)
	}
}
