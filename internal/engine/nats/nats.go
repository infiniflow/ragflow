//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package nats

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type NatsEngine struct {
	host      string
	port      int
	jetStream jetstream.JetStream
	stream    jetstream.Stream
}

func NewNatsEngine(host string, port int) *NatsEngine {
	return &NatsEngine{
		host: host,
		port: port,
	}
}

func (n *NatsEngine) Init() error {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	n.jetStream, err = jetstream.New(nc)
	if err != nil {
		nc.Close()
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	streamCfg := jetstream.StreamConfig{
		Name:      "RAGFLOW_TASKS",
		Subjects:  []string{"tasks.>"},
		Retention: jetstream.WorkQueuePolicy,
		Storage:   jetstream.FileStorage,
		MaxMsgs:   1024 * 128,
		MaxBytes:  1024 * 1024,
	}

	n.stream, err = n.jetStream.CreateStream(ctx, streamCfg)
	if err != nil {
		if err.Error() != "stream already exists" {
			nc.Close()
			return fmt.Errorf("fail to create stream: %w", err)
		}

		common.Info("NATS stream already exists, use existing stream")
		n.stream, err = n.jetStream.Stream(ctx, "RAGFLOW_TASKS")
		if err != nil {
			nc.Close()
			return fmt.Errorf("fail to get existing stream: %w", err)
		}
	} else {
		common.Info("NATS stream create successfully")
	}

	return nil
}

func (n *NatsEngine) PublishTask(subject string, payload []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ack, err := n.jetStream.Publish(ctx, subject, payload)
	if err != nil {
		return err
	}
	common.Info(fmt.Sprintf("Task published, stream seq: %d", ack.Sequence))
	return nil
}

func (n *NatsEngine) ListMessages(messageType string) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if n.stream == nil {
		return nil, fmt.Errorf("NATS stream not initialized")
	}

	subjectFilter := "tasks.>"

	info, err := n.stream.Info(ctx, jetstream.WithSubjectFilter(subjectFilter))
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	if info.State.Msgs == 0 {
		return nil, nil
	}

	var messages []map[string]string
	seq := info.State.FirstSeq
	lastSeq := info.State.LastSeq

	for seq <= lastSeq {
		var msg *jetstream.RawStreamMsg
		msg, err = n.stream.GetMsg(ctx, seq, jetstream.WithGetMsgSubject(subjectFilter))
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgNotFound) {
				break
			}
			return nil, fmt.Errorf("failed to get message at seq %d: %w", seq, err)
		}
		messageMap := make(map[string]string)
		messageMap["subject"] = msg.Subject
		messageMap["message"] = string(msg.Data)
		messages = append(messages, messageMap)
		seq = msg.Sequence + 1
	}

	common.Info(fmt.Sprintf("Listed %d messages for subject: %s", len(messages), subjectFilter))
	return messages, nil
}
func (n *NatsEngine) ConsumeMessage(subject string, messageCount int) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	consumer, err := n.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          "RAGFLOW_CONSUMER",
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    3,
		MaxAckPending: 100,
		FilterSubject: "tasks.>",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Consumer: %w", err)
	}
	//c.consumer = consumer
	resultMessages := make([]map[string]string, 0)
	messages, err := consumer.Fetch(messageCount, jetstream.FetchMaxWait(1*time.Second))
	for msg := range messages.Messages() {
		messageMap := make(map[string]string)
		messageMap["subject"] = msg.Subject()
		messageMap["message"] = string(msg.Data())
		common.Debug(fmt.Sprintf("New message: %s", string(msg.Data())))
		err = msg.Ack()
		if err != nil {
			return nil, err
		}
		resultMessages = append(resultMessages, messageMap)
	}

	return resultMessages, nil
}
