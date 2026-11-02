package service

import (
	"encoding/json"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/engine"
)

type TaskPublisher interface {
	PublishTaskMessage(subject string, msg common.TaskMessage) error
}

type MessageQueueTaskPublisher struct {
	// mqEngine is the injected MQ engine. A nil value falls back to the
	// process-wide engine.GetMessageQueueEngine() for legacy callers that have
	// not migrated to injection. The memory module always injects, so its
	// publish path never reads the global.
	mqEngine engine.MessageQueue
}

func NewMessageQueueTaskPublisher() *MessageQueueTaskPublisher {
	return &MessageQueueTaskPublisher{}
}

// NewMessageQueueTaskPublisherWithEngine builds a publisher backed by the
// injected MQ engine. The memory module uses this so its publish path does not
// reach the process-wide engine.GetMessageQueueEngine() global.
func NewMessageQueueTaskPublisherWithEngine(mq engine.MessageQueue) *MessageQueueTaskPublisher {
	return &MessageQueueTaskPublisher{mqEngine: mq}
}

func (p *MessageQueueTaskPublisher) PublishTaskMessage(subject string, msg common.TaskMessage) error {
	// Prefer the injected engine; fall back to the process-wide global only for
	// legacy callers that construct the publisher without injection. The memory
	// module always injects, so its publish path never reads the global.
	msgQueueEngine := p.mqEngine
	if msgQueueEngine == nil {
		msgQueueEngine = engine.GetMessageQueueEngine()
	}
	if msgQueueEngine == nil {
		return fmt.Errorf("message queue engine is not initialized")
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return msgQueueEngine.PublishTask(subject, payload)
}
