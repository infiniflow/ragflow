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

type MessageQueueTaskPublisher struct{}

func NewMessageQueueTaskPublisher() *MessageQueueTaskPublisher {
	return &MessageQueueTaskPublisher{}
}

func (p *MessageQueueTaskPublisher) PublishTaskMessage(subject string, msg common.TaskMessage) error {
	msgQueueEngine := engine.GetMessageQueueEngine()
	if msgQueueEngine == nil {
		return fmt.Errorf("message queue engine is not initialized")
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return msgQueueEngine.PublishTask(subject, payload)
}
