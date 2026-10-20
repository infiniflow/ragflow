package service

import (
	"encoding/json"
	"sync"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
)

// recordingMessageQueue is a fake engine.MessageQueue that records publishes.
// It embeds the interface so it satisfies MessageQueue; only PublishTask is
// overridden. Other methods are never called by the publisher under test.
type recordingMessageQueue struct {
	engine.MessageQueue
	mu       sync.Mutex
	subjects []string
	messages []common.TaskMessage
}

func (r *recordingMessageQueue) PublishTask(subject string, payload []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subjects = append(r.subjects, subject)
	var m common.TaskMessage
	_ = json.Unmarshal(payload, &m)
	r.messages = append(r.messages, m)
	return nil
}

// TestMessageQueueTaskPublisherUsesInjectedEngineNotGlobal verifies the
// injected engine wins over the process-wide global: publishing through a
// publisher built with NewMessageQueueTaskPublisherWithEngine must hit the
// injected engine and must NOT touch engine.GetMessageQueueEngine(). This is
// the closed-DI guarantee for the memory module's publish path (F1).
func TestMessageQueueTaskPublisherUsesInjectedEngineNotGlobal(t *testing.T) {
	globalMQ := &recordingMessageQueue{}
	engine.SetMessageQueueEngine(globalMQ)
	defer engine.SetMessageQueueEngine(nil)

	injectedMQ := &recordingMessageQueue{}
	pub := NewMessageQueueTaskPublisherWithEngine(injectedMQ)

	if err := pub.PublishTaskMessage(common.TaskSubject, common.TaskMessage{
		TaskID:   "task-1",
		TaskType: common.TaskTypeMemory,
	}); err != nil {
		t.Fatalf("PublishTaskMessage: %v", err)
	}

	if len(injectedMQ.messages) != 1 || injectedMQ.messages[0].TaskID != "task-1" {
		t.Fatalf("injected engine got %+v, want one publish for task-1", injectedMQ.messages)
	}
	if len(globalMQ.messages) != 0 {
		t.Fatalf("global engine was used (%d publishes); injected engine should win", len(globalMQ.messages))
	}
}

// TestMemoryMessageServicePublishesViaInjectedEngine verifies the inheritance
// chain MemoryService.WithMemoryMQEngine -> MemoryMessageService -> its
// publisher uses the injected engine, not the global (F1, service level).
func TestMemoryMessageServicePublishesViaInjectedEngine(t *testing.T) {
	globalMQ := &recordingMessageQueue{}
	engine.SetMessageQueueEngine(globalMQ)
	defer engine.SetMessageQueueEngine(nil)

	injectedMQ := &recordingMessageQueue{}
	memSvc := NewMemoryService(nil, nil, WithMemoryMQEngine(injectedMQ))
	svc := NewMemoryMessageService(memSvc)

	if svc.taskPublisher == nil {
		t.Fatal("MemoryMessageService.taskPublisher is nil")
	}
	if err := publishMemoryTaskWakeup(svc.taskPublisher, "task-1"); err != nil {
		t.Fatalf("publishMemoryTaskWakeup: %v", err)
	}
	if len(injectedMQ.messages) != 1 {
		t.Fatalf("injected engine got %d publishes, want 1", len(injectedMQ.messages))
	}
	if len(globalMQ.messages) != 0 {
		t.Fatalf("global engine was used; injected engine should win")
	}
}

// TestMessageQueueTaskPublisherNilEngineFallsBackToGlobal verifies the legacy
// no-arg constructor still resolves through the global when no engine is
// injected, so non-memory callers (out of the memory module's DI scope) keep
// working.
func TestMessageQueueTaskPublisherNilEngineFallsBackToGlobal(t *testing.T) {
	globalMQ := &recordingMessageQueue{}
	engine.SetMessageQueueEngine(globalMQ)
	defer engine.SetMessageQueueEngine(nil)

	pub := NewMessageQueueTaskPublisher()
	if err := pub.PublishTaskMessage(common.TaskSubject, common.TaskMessage{
		TaskID:   "task-2",
		TaskType: common.TaskTypeMemory,
	}); err != nil {
		t.Fatalf("PublishTaskMessage: %v", err)
	}
	if len(globalMQ.messages) != 1 {
		t.Fatalf("global fallback got %d publishes, want 1", len(globalMQ.messages))
	}
}
