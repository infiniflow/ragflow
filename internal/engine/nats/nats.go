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
	"encoding/json"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type NatsEngine struct {
	host      string
	port      int
	nc        *nats.Conn
	jetStream jetstream.JetStream
	stream    jetstream.Stream
	consumer  jetstream.Consumer
}

func NewNatsEngine(host string, port int) *NatsEngine {
	return &NatsEngine{
		host: host,
		port: port,
	}
}

func (n *NatsEngine) Init() error {
	var err error
	natsURL := fmt.Sprintf("nats://%s:%d", n.host, n.port)
	n.nc, err = nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS at %s: %w", natsURL, err)
	}

	n.jetStream, err = jetstream.New(n.nc)
	if err != nil {
		n.nc.Close()
		return fmt.Errorf("failed to create JetStream context at %s: %w", natsURL, err)
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
		if !strings.Contains(err.Error(), "already exists") {
			n.nc.Close()
			return fmt.Errorf("fail to create stream at %s: %w", natsURL, err)
		}

		common.Info("NATS stream already exists, use existing stream")
		n.stream, err = n.jetStream.Stream(ctx, "RAGFLOW_TASKS")
		if err != nil {
			n.nc.Close()
			return fmt.Errorf("fail to get existing stream at %s: %w", natsURL, err)
		}
	} else {
		common.Info(fmt.Sprintf("NATS stream create successfully at %s", natsURL))
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

func (n *NatsEngine) ShowMessageQueue() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	accountInfo, err := n.jetStream.AccountInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}
	result := make(map[string]string)
	result["consumer_count"] = strconv.Itoa(accountInfo.Consumers)
	result["memory"] = strconv.FormatUint(accountInfo.Memory, 10)

	subjectFilter := "tasks.>"
	info, err := n.stream.Info(ctx, jetstream.WithSubjectFilter(subjectFilter))
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}
	result["message_count"] = strconv.FormatUint(info.State.Msgs, 10)

	consumer, err := n.stream.Consumer(ctx, "RAGFLOW_CONSUMER")
	if err != nil {
		return nil, fmt.Errorf("failed to get existing consumer: %w", err)
	}

	consumerInfo, err := consumer.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer info: %w", err)
	}
	result["pending_count"] = strconv.FormatUint(consumerInfo.NumPending, 10)
	result["waiting_count"] = strconv.Itoa(consumerInfo.NumWaiting)
	result["ack_pending_count"] = strconv.Itoa(consumerInfo.NumAckPending)
	result["redelivered_count"] = strconv.Itoa(consumerInfo.NumRedelivered)
	return result, nil
}

func (n *NatsEngine) ListMessages(messageType string, pending bool) ([]map[string]string, error) {
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

func (n *NatsEngine) InitConsumer(subject string) error {
	if n.stream == nil {
		return fmt.Errorf("NATS stream is nil, engine not properly initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	n.consumer, err = n.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          "RAGFLOW_CONSUMER",
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    16,
		MaxAckPending: 1024 * 128,
		FilterSubject: "tasks.>",
	})
	if err != nil {
		// MaxAckPending is immutable after consumer creation.
		// If the consumer already exists, fall back to fetching it.
		if strings.Contains(err.Error(), "max waiting can not be updated") {
			n.consumer, err = n.stream.Consumer(ctx, "RAGFLOW_CONSUMER")
			if err != nil {
				return fmt.Errorf("failed to get existing consumer: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create Consumer: %w", err)
		}
	}
	return nil
}
func (n *NatsEngine) GetMessages(messageCount int) ([]common.TaskHandle, error) {
	resultMessages := make([]common.TaskHandle, 0)
	messages, err := n.consumer.Fetch(messageCount, jetstream.FetchMaxWait(1*time.Second))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}
	for msg := range messages.Messages() {
		resultMessages = append(resultMessages, NewNatsMessageHandle(msg))
	}
	return resultMessages, nil
}

func (n *NatsEngine) CheckStatus() string {
	n.nc.Stats()
	return n.nc.Status().String()
}

type NatsMessageHandle struct {
	message jetstream.Msg
}

func NewNatsMessageHandle(message jetstream.Msg) *NatsMessageHandle {
	return &NatsMessageHandle{
		message: message,
	}
}

func (m *NatsMessageHandle) GetMessage() common.TaskMessage {
	// convert to task message
	var taskMessage common.TaskMessage
	if err := json.Unmarshal(m.message.Data(), &taskMessage); err != nil {
		common.Error("failed to unmarshal message", err)
	}
	return taskMessage
}

func (m *NatsMessageHandle) Ack() error {
	return m.message.Ack()
}

func (m *NatsMessageHandle) Nack() error {
	return m.message.Nak()
}
