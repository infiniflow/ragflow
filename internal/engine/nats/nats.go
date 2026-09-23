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
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// isStreamExistsErr reports whether err means the stream already exists, using
// the typed JetStream APIError (err_code 10058) rather than error-string
// matching. The APIError code is the canonical, stable signal from the server.
func isStreamExistsErr(err error) bool {
	if errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return true
	}
	var apiErr *jetstream.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode == jetstream.JSErrCodeStreamNameInUse
	}
	return false
}

type NatsEngine struct {
	host      string
	port      int
	nc        *nats.Conn
	jetStream jetstream.JetStream
	stream    jetstream.Stream
	// consumer is the task consumer handle. Guarded by consumerMu: the pull loop
	// and the admin inspection endpoint share one engine, and the handle is
	// replaced when it is found to have outlived its consumer (see PullMessages).
	consumerMu sync.Mutex
	consumer   jetstream.Consumer

	// dataset-level compile consumer (§11) state.
	knowledgeCompileStream   jetstream.Stream
	knowledgeCompileConsumer jetstream.Consumer
	kv                       jetstream.KeyValue

	syncerStream     jetstream.Stream
	syncerConsumer   jetstream.PushConsumer
	syncCheckpointKV jetstream.KeyValue
	syncerMu         sync.Mutex
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
		Discard:   jetstream.DiscardNew,
		MaxMsgs:   1024 * 1024,
		MaxBytes:  1024 * 1024 * 1024,
	}

	n.stream, err = ensureStreamConfig(ctx, n.jetStream, streamCfg)
	if err != nil {
		if !isStreamExistsErr(err) {
			n.nc.Close()
			return fmt.Errorf("fail to create stream at %s: %w", natsURL, err)
		}

		common.Info("NATS stream already exists, use existing stream")
		n.stream, err = n.jetStream.Stream(ctx, "RAGFLOW_TASKS")
		if err != nil {
			n.nc.Close()
			return fmt.Errorf("fail to get existing stream at %s: %w", natsURL, err)
		}
		// Reconcile the running stream with the configured limits so an existing
		// stream created under older settings (e.g. DiscardOld + small MaxBytes)
		// picks up DiscardNew and the larger MaxMsgs/MaxBytes. CreateStream never
		// touches an already-existing stream, so UpdateStream is required here.
		n.stream, err = n.jetStream.UpdateStream(ctx, streamCfg)
		if err != nil {
			n.nc.Close()
			return fmt.Errorf("fail to update existing stream at %s: %w", natsURL, err)
		}
		common.Info("NATS stream updated with current config (discard=new, larger limits)")
	} else {
		common.Info("NATS stream create successfully")
	}
	common.Info(fmt.Sprintf("NATS stream RAGFLOW_TASKS ready at %s", natsURL))

	return nil
}

func (n *NatsEngine) Type() string {
	return "nats"
}

func (n *NatsEngine) PublishTask(subject string, payload []byte) error {
	if n.jetStream == nil {
		return errors.New("NATS jetstream is nil, engine not properly initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Deliberately NO MsgID/dedup here. Ingestion tasks reuse the same
	// task_id across publish attempts (the FAILED/STOPPED→CREATED→SCHEDULED retry
	// path), and the server's Duplicates window suppresses a republished
	// MsgID for its whole lifetime regardless of whether the original
	// message was already consumed and acked. A deduped retry publish
	// strands the task in SCHEDULED with no message behind it — unreachable
	// by any consumer and un-reparsable ("already exists, status: SCHEDULED").
	// Duplicate delivery is instead made safe at the consumer level:
	// StartRunning's CREATED/SCHEDULED→RUNNING CAS plus the in-process claim guard
	// prevent a second copy from executing while the first owner is active (see
	// Ingestor.handleAndExecute).
	ack, err := n.jetStream.Publish(ctx, subject, payload)
	if err != nil {
		return err
	}
	common.Info(fmt.Sprintf("Task published, stream seq: %d", ack.Sequence))
	return nil
}

func (n *NatsEngine) ShowMessageQueue() (map[string]string, error) {
	if n.jetStream == nil || n.stream == nil {
		return nil, errors.New("NATS jetstream/stream is nil, engine not properly initialized")
	}

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
	if err == nil {
		var consumerInfo *jetstream.ConsumerInfo
		consumerInfo, err = consumer.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get consumer info: %w", err)
		}
		result["pending_count"] = strconv.FormatUint(consumerInfo.NumPending, 10)
		result["waiting_count"] = strconv.Itoa(consumerInfo.NumWaiting)
		result["ack_pending_count"] = strconv.Itoa(consumerInfo.NumAckPending)
		result["redelivered_count"] = strconv.Itoa(consumerInfo.NumRedelivered)
	}

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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return n.ensureConsumer(ctx)
}

// ensureConsumer creates or updates RAGFLOW_CONSUMER and stores the handle.
// It is idempotent, so it doubles as the repair path when a stored handle is
// found to have outlived its consumer. The consumer name is a fixed, durable
// name on purpose: a restart must reattach to the same consumer (and its
// pending messages) instead of leaving an orphan behind.
func (n *NatsEngine) ensureConsumer(ctx context.Context) error {
	if n.stream == nil {
		return fmt.Errorf("NATS stream is nil, engine not properly initialized")
	}

	// Explicit redelivery schedule: BackOff paces successive redeliveries
	// (5s/15s/30s, then 60s repeated) so an unsettled message (crash, slow
	// DB) is retried with breathing room instead of the broker default. The
	// server normalizes AckWait to BackOff[0] when BackOff is present; the
	// 60s AckWait is the effective schedule if BackOff is ever dropped.
	// INVARIANT: the worker's InProgress heartbeat (Ingestor
	// defaultHeartbeatInterval) must stay below BackOff[0] = 5s, or in-flight
	// messages get redelivered mid-run before the owning worker can settle them.
	// Note: CreateOrUpdateConsumer is atomic. MaxWaiting is immutable after
	// creation; if it mismatches the whole update fails (error "max waiting
	// can not be updated") and NONE of AckWait/BackOff/MaxAckPending are
	// applied. When MaxWaiting matches (the default, since this config omits
	// it), the other three update in place. The fallback below handles the
	// MaxWaiting mismatch by keeping the existing consumer.
	consumer, err := n.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          "RAGFLOW_CONSUMER",
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    16,
		MaxAckPending: 1024 * 128,
		FilterSubject: "tasks.>",
		AckWait:       60 * time.Second,
		BackOff:       []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second},
	})
	if err != nil {
		// MaxWaiting is immutable after consumer creation (AckWait/BackOff/MaxAckPending remain mutable when MaxWaiting matches; the update is atomic).
		// If the consumer already exists, fall back to fetching it.
		if strings.Contains(err.Error(), "max waiting can not be updated") {
			consumer, err = n.stream.Consumer(ctx, "RAGFLOW_CONSUMER")
			if err != nil {
				return fmt.Errorf("failed to get existing consumer: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create Consumer: %w", err)
		}
	}
	n.consumerMu.Lock()
	n.consumer = consumer
	n.consumerMu.Unlock()
	return nil
}

// consumerHandle returns the task consumer handle, or an error when the engine
// has not created one yet.
func (n *NatsEngine) consumerHandle() (jetstream.Consumer, error) {
	n.consumerMu.Lock()
	defer n.consumerMu.Unlock()
	if n.consumer == nil {
		return nil, errors.New("NATS consumer is nil, engine not properly initialized")
	}
	return n.consumer, nil
}

// recreateConsumer rebuilds RAGFLOW_CONSUMER and returns the fresh handle.
// Consumer creation is a JetStream API round trip, so it gets its own timeout
// rather than borrowing the (deliberately short) pull deadline.
func (n *NatsEngine) recreateConsumer(ctx context.Context) (jetstream.Consumer, error) {
	createCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := n.ensureConsumer(createCtx); err != nil {
		return nil, err
	}
	return n.consumerHandle()
}

// consumerRepairPullAttempts / consumerRepairPullDelay bound the retry that
// follows a consumer rebuild: the pull subject of a freshly created consumer can
// take a moment to become addressable, and a pull in that window answers the
// very same "no responders" error the repair exists to clear.
const (
	consumerRepairPullAttempts = 5
	consumerRepairPullDelay    = 200 * time.Millisecond
)

// isConsumerGoneErr reports whether err means the stored consumer handle no
// longer addresses a live consumer: it was deleted, never existed, or the
// JetStream context went stale. Typed JetStream/NATS errors are checked first -
// they are the stable signal - with a string fallback for wrapped variants.
func isConsumerGoneErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, jetstream.ErrConsumerNotFound) ||
		errors.Is(err, jetstream.ErrConsumerDeleted) ||
		errors.Is(err, nats.ErrNoResponders) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no responders") ||
		strings.Contains(msg, "consumer not found") ||
		strings.Contains(msg, "consumer deleted")
}

// PullMessages fetches up to messageCount messages before ctx expires.
func (n *NatsEngine) PullMessages(ctx context.Context, messageCount int) ([]common.TaskHandle, error) {
	if messageCount < 1 || messageCount > common.MaxManualPullMessages {
		return nil, fmt.Errorf("message count must be between 1 and %d", common.MaxManualPullMessages)
	}
	consumer, err := n.consumerHandle()
	if err != nil {
		return nil, err
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("pull messages context must have a deadline")
	}

	resultMessages, pullErr := n.fetchOnce(ctx, consumer, messageCount)
	if !isConsumerGoneErr(pullErr) {
		if pullErr != nil {
			return nil, fmt.Errorf("failed to fetch messages: %w", pullErr)
		}
		return resultMessages, nil
	}

	// The handle outlived its consumer (deleted behind our back, or the
	// JetStream context went stale across a reconnect). Retrying against the
	// dead handle answers "no responders available for request" forever: the
	// worker logs errors, the queue stops draining, and only a process restart
	// clears it. Rebuild the consumer and retry, bounded, so a persistent failure
	// still surfaces instead of spinning.
	common.Warn(fmt.Sprintf("NATS consumer went missing (%v); recreating RAGFLOW_CONSUMER", pullErr))
	consumer, recreateErr := n.recreateConsumer(ctx)
	if recreateErr != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w (recreate consumer: %v)", pullErr, recreateErr)
	}
	// A pull issued in the instant right after the consumer is created can still
	// land before the server registers the new consumer's pull subject - the same
	// "no responders" answer, briefly - so retry a few times with a short pause
	// instead of declaring the repair failed.
	for attempt := 1; attempt <= consumerRepairPullAttempts; attempt++ {
		resultMessages, pullErr = n.fetchOnce(ctx, consumer, messageCount)
		if !isConsumerGoneErr(pullErr) || attempt == consumerRepairPullAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("failed to fetch messages: %w", pullErr)
		case <-time.After(consumerRepairPullDelay):
		}
	}
	if pullErr != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", pullErr)
	}
	return resultMessages, nil
}

// fetchOnce performs a single pull and folds the batch error into the returned
// error. A pull deadline means "nothing was delivered" and is not an error - the
// caller's loop relies on that - while any other batch failure nacks whatever did
// arrive so the messages stay available. Errors can surface either from Fetch
// itself or from the returned batch, which is why the no-responders repair above
// has to look at both.
func (n *NatsEngine) fetchOnce(ctx context.Context, consumer jetstream.Consumer, messageCount int) ([]common.TaskHandle, error) {
	messages, err := consumer.Fetch(messageCount, jetstream.FetchContext(ctx))
	if err != nil {
		return nil, err
	}
	resultMessages := make([]common.TaskHandle, 0, messageCount)
	for message := range messages.Messages() {
		resultMessages = append(resultMessages, NewNatsMessageHandle(message))
	}
	if batchErr := messages.Error(); batchErr != nil {
		if errors.Is(batchErr, context.DeadlineExceeded) {
			return resultMessages, nil
		}
		for _, message := range resultMessages {
			if nackErr := message.Nack(); nackErr != nil {
				common.Error("nack message after failed pull", nackErr)
			}
		}
		return nil, batchErr
	}
	return resultMessages, nil
}

// PullMessage returns one task handle from PullMessages. A nil handle
// with a nil error means the pull expired without an available task.
func (n *NatsEngine) PullMessage(ctx context.Context) (common.TaskHandle, error) {
	messages, err := n.PullMessages(ctx, 1)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	return messages[0], nil
}

func (n *NatsEngine) CheckStatus() string {
	if n.nc == nil {
		return "NATS connection is nil, engine not properly initialized"
	}
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

func (m *NatsMessageHandle) InProgress() error {
	return m.message.InProgress()
}
