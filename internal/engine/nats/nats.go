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

// tierStreamSpec describes one ingestion lane's stream, subjects and durable
// consumer. The set of lanes (tierOrder / tierStreams) and the "default" lane
// (defaultTier) are edition-specific and live in build-tagged files
// (tier_policy_ee.go / tier_policy_community.go) so the open-source split
// receives exactly one queue and no enterprise tier names, while the methods
// below stay identical across both editions.
type tierStreamSpec struct {
	streamName   string
	subjects     []string
	consumerName string
}

// tierFetchWindow is the per-tier pull budget. PullMessage gives each lane its
// own window in priority order, so several empty lanes cost at most Nx this
// budget while the caller's context bounds the whole scan.
const tierFetchWindow = 1 * time.Second

type NatsEngine struct {
	host      string
	port      int
	nc        *nats.Conn
	jetStream jetstream.JetStream

	streamMu sync.Mutex
	streams  map[common.Tier]jetstream.Stream
	// stream is the default stream handle (the lane returned by defaultTier).
	// Admin inspection endpoints read this.
	stream jetstream.Stream

	// consumer is the default consumer handle (the lane returned by
	// defaultTier), guarded by consumerMu. consumers holds every lane; the pull
	// loop replaces a lane's handle when that lane's consumer is found to have
	// gone missing.
	consumerMu sync.Mutex
	consumers  map[common.Tier]jetstream.Consumer
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

	streams := make(map[common.Tier]jetstream.Stream, len(tierOrder))
	for _, tier := range tierOrder {
		spec := tierStreams[tier]
		streamCfg := jetstream.StreamConfig{
			Name:      spec.streamName,
			Subjects:  spec.subjects,
			Retention: jetstream.WorkQueuePolicy,
			Storage:   jetstream.FileStorage,
			Discard:   jetstream.DiscardNew,
			MaxMsgs:   1024 * 1024,
			MaxBytes:  1024 * 1024 * 1024,
		}
		// ensureStreamConfig creates the stream, or migrates an already-existing
		// one in place (DiscardOld/small MaxBytes -> DiscardNew/larger cap) while
		// preserving fields it does not own. It never returns "already exists".
		stream, streamErr := ensureStreamConfig(ctx, n.jetStream, streamCfg)
		if streamErr != nil {
			n.nc.Close()
			return fmt.Errorf("fail to prepare stream %s at %s: %w", spec.streamName, natsURL, streamErr)
		}
		streams[tier] = stream
		common.Info(fmt.Sprintf("NATS stream %s ready at %s", spec.streamName, natsURL))
	}

	n.streamMu.Lock()
	n.streams = streams
	n.stream = streams[defaultTier()]
	n.streamMu.Unlock()

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

	// Aggregate the edition-specific physically isolated lanes (the enterprise
	// build defines multiple lanes, OSS uses a single lane), and also expose
	// per-tier counts so an operator can see where back-pressure is building up.
	var totalMsgs uint64
	var totalPending uint64
	var totalWaiting, totalAckPending, totalRedelivered int
	for _, tier := range tierOrder {
		spec := tierStreams[tier]
		n.streamMu.Lock()
		stream := n.streams[tier]
		n.streamMu.Unlock()
		if stream == nil {
			continue
		}
		info, err := stream.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get stream info for %s: %w", spec.streamName, err)
		}
		totalMsgs += info.State.Msgs
		result[string(tier)+"_message_count"] = strconv.FormatUint(info.State.Msgs, 10)

		consumer, err := stream.Consumer(ctx, spec.consumerName)
		if err != nil {
			continue
		}
		consumerInfo, err := consumer.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get consumer info for %s: %w", spec.consumerName, err)
		}
		totalPending += consumerInfo.NumPending
		totalWaiting += consumerInfo.NumWaiting
		totalAckPending += consumerInfo.NumAckPending
		totalRedelivered += consumerInfo.NumRedelivered
	}
	result["message_count"] = strconv.FormatUint(totalMsgs, 10)
	result["pending_count"] = strconv.FormatUint(totalPending, 10)
	result["waiting_count"] = strconv.Itoa(totalWaiting)
	result["ack_pending_count"] = strconv.Itoa(totalAckPending)
	result["redelivered_count"] = strconv.Itoa(totalRedelivered)

	return result, nil
}

func (n *NatsEngine) ListMessages(messageType string, pending bool) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	n.streamMu.Lock()
	streams := make(map[common.Tier]jetstream.Stream, len(n.streams))
	for tier, stream := range n.streams {
		streams[tier] = stream
	}
	n.streamMu.Unlock()
	if len(streams) == 0 {
		return nil, fmt.Errorf("NATS stream not initialized")
	}

	// Walk every lane in priority order. Each stream owns its own subject set,
	// so no subject filter is needed to avoid cross-lane contamination.
	var messages []map[string]string
	for _, tier := range tierOrder {
		stream := streams[tier]
		if stream == nil {
			continue
		}
		info, err := stream.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get stream info for tier %s: %w", tier, err)
		}
		if info.State.Msgs == 0 {
			continue
		}
		seq := info.State.FirstSeq
		lastSeq := info.State.LastSeq
		for seq <= lastSeq {
			var msg *jetstream.RawStreamMsg
			msg, err = stream.GetMsg(ctx, seq)
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
	}

	common.Info(fmt.Sprintf("Listed %d messages across %d tier streams", len(messages), len(streams)))
	return messages, nil
}

// InitConsumer creates (or reattaches to) the edition-specific durable tier
// consumers — three (high/medium/low) in EE, one in the community build. The
// subject argument is retained for API compatibility with the pre-tiering
// single-consumer contract and is intentionally ignored: each lane now owns a
// durable consumer on its own stream.
func (n *NatsEngine) InitConsumer(subject string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return n.ensureConsumers(ctx)
}

// ensureConsumers creates or updates one durable consumer per tier stream and
// stores every handle (high/medium/low in EE, one in OSS). It is idempotent, so
// it doubles as the repair path when a stored handle is found to have outlived
// its consumer. Consumer names
// are fixed and durable: a restart must reattach to the same consumers (and
// their pending messages) instead of leaving orphans behind.
func (n *NatsEngine) ensureConsumers(ctx context.Context) error {
	consumers := make(map[common.Tier]jetstream.Consumer, len(tierOrder))
	for _, tier := range tierOrder {
		consumer, err := n.ensureConsumer(ctx, tier)
		if err != nil {
			return err
		}
		consumers[tier] = consumer
	}
	n.consumerMu.Lock()
	n.consumers = consumers
	n.consumer = consumers[defaultTier()]
	n.consumerMu.Unlock()
	return nil
}

// ensureConsumer creates or updates a single lane's durable consumer.
func (n *NatsEngine) ensureConsumer(ctx context.Context, tier common.Tier) (jetstream.Consumer, error) {
	n.streamMu.Lock()
	stream := n.streams[tier]
	n.streamMu.Unlock()
	if stream == nil {
		return nil, fmt.Errorf("NATS stream for tier %s is nil, engine not properly initialized", tier)
	}
	spec := tierStreams[tier]

	// Explicit redelivery schedule: BackOff paces successive redeliveries
	// (5s/15s/30s, then 60s repeated) so an unsettled message (crash, slow
	// DB) is retried with breathing room instead of the broker default.
	// INVARIANT: the worker's InProgress heartbeat must stay below BackOff[0]
	// = 5s, or in-flight messages get redelivered mid-run.
	// MaxWaiting is immutable after consumer creation; the fallback below keeps
	// an existing consumer whose MaxWaiting differs.
	consumerCfg := jetstream.ConsumerConfig{
		Name:          spec.consumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    16,
		MaxAckPending: 1024 * 128,
		AckWait:       60 * time.Second,
		BackOff:       []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second},
	}
	// Each lane currently owns exactly one subject, so the singular
	// FilterSubject path is taken. FilterSubjects (plural) is retained as a
	// defensive branch for a future lane that listens on more than one subject;
	// it requires nats-server >= 2.10 and is exclusive with FilterSubject.
	if len(spec.subjects) == 1 {
		consumerCfg.FilterSubject = spec.subjects[0]
	} else {
		consumerCfg.FilterSubjects = spec.subjects
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, consumerCfg)
	if err != nil {
		if strings.Contains(err.Error(), "max waiting can not be updated") {
			consumer, err = stream.Consumer(ctx, spec.consumerName)
			if err != nil {
				return nil, fmt.Errorf("failed to get existing consumer %s: %w", spec.consumerName, err)
			}
		} else {
			return nil, fmt.Errorf("failed to create consumer %s: %w", spec.consumerName, err)
		}
	}
	return consumer, nil
}

// consumerHandle returns one lane's consumer handle, or an error when the
// engine has not created it yet.
func (n *NatsEngine) consumerHandle(tier common.Tier) (jetstream.Consumer, error) {
	n.consumerMu.Lock()
	defer n.consumerMu.Unlock()
	consumer := n.consumers[tier]
	if consumer == nil {
		return nil, fmt.Errorf("NATS consumer for tier %s is nil, engine not properly initialized", tier)
	}
	return consumer, nil
}

// recreateConsumer rebuilds one lane's consumer and returns the fresh handle.
// Consumer creation is a JetStream API round trip, so it gets its own timeout
// rather than borrowing the (deliberately short) pull deadline. Only the lane
// that went missing is rebuilt; the other lanes keep draining.
func (n *NatsEngine) recreateConsumer(ctx context.Context, tier common.Tier) (jetstream.Consumer, error) {
	createCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	consumer, err := n.ensureConsumer(createCtx, tier)
	if err != nil {
		return nil, err
	}
	n.consumerMu.Lock()
	if n.consumers == nil {
		n.consumers = make(map[common.Tier]jetstream.Consumer)
	}
	n.consumers[tier] = consumer
	if tier == defaultTier() {
		n.consumer = consumer
	}
	n.consumerMu.Unlock()
	return consumer, nil
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

// nackAll returns every handle in the slice to its lane so they are redelivered
// instead of hanging until AckWait/BackOff expires. It is used when PullMessages
// bails out after a lane already yielded messages: those earlier handles must
// not be stranded just because a later lane failed. Nack failures are logged
// and swallowed — a single bad handle must not block the others.
func nackAll(handles []common.TaskHandle) {
	for _, h := range handles {
		if err := h.Nack(); err != nil {
			common.Warn(fmt.Sprintf("NATS: failed to nack a drained message: %v", err))
		}
	}
}

// pullLane fetches up to messageCount messages from one lane. Each lane owns a
// short independent window derived from ctx, so an empty lane cannot eat the
// whole budget before the others get a turn. If the lane's consumer handle has
// gone stale, only that lane is rebuilt and retried.
func (n *NatsEngine) pullLane(ctx context.Context, tier common.Tier, messageCount int) ([]common.TaskHandle, error) {
	consumer, err := n.consumerHandle(tier)
	if err != nil {
		return nil, err
	}
	laneCtx, cancel := context.WithTimeout(ctx, tierFetchWindow)
	defer cancel()

	resultMessages, pullErr := n.fetchOnce(laneCtx, consumer, messageCount)
	if !isConsumerGoneErr(pullErr) {
		if pullErr != nil {
			return nil, fmt.Errorf("failed to fetch messages from tier %s: %w", tier, pullErr)
		}
		return resultMessages, nil
	}

	// The handle outlived its consumer. Rebuild ONLY this lane's consumer and
	// retry, bounded, so a persistent failure still surfaces instead of
	// spinning; the other lanes keep their handles and keep draining.
	common.Warn(fmt.Sprintf("NATS consumer for tier %s went missing (%v); recreating", tier, pullErr))
	consumer, recreateErr := n.recreateConsumer(ctx, tier)
	if recreateErr != nil {
		return nil, fmt.Errorf("failed to fetch messages from tier %s: %w (recreate consumer: %v)", tier, pullErr, recreateErr)
	}
	for attempt := 1; attempt <= consumerRepairPullAttempts; attempt++ {
		retryCtx, retryCancel := context.WithTimeout(ctx, tierFetchWindow)
		resultMessages, pullErr = n.fetchOnce(retryCtx, consumer, messageCount)
		retryCancel()
		if !isConsumerGoneErr(pullErr) || attempt == consumerRepairPullAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("failed to fetch messages from tier %s: %w", tier, pullErr)
		case <-time.After(consumerRepairPullDelay):
		}
	}
	if pullErr != nil {
		return nil, fmt.Errorf("failed to fetch messages from tier %s: %w", tier, pullErr)
	}
	return resultMessages, nil
}

// PullMessages fetches up to messageCount messages, draining lanes in the
// edition's configured priority order. It stops as soon as the batch is full.
func (n *NatsEngine) PullMessages(ctx context.Context, messageCount int) ([]common.TaskHandle, error) {
	if messageCount < 1 || messageCount > common.MaxManualPullMessages {
		return nil, fmt.Errorf("message count must be between 1 and %d", common.MaxManualPullMessages)
	}
	// Fail fast on a half-initialized engine before the deadline requirement,
	// so an uninitialized engine always reports "not properly initialized"
	// regardless of the caller's context (mirrors PublishTask/ShowMessageQueue).
	if _, err := n.consumerHandle(defaultTier()); err != nil {
		return nil, err
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("pull messages context must have a deadline")
	}

	result := make([]common.TaskHandle, 0, messageCount)
	for _, tier := range tierOrder {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				nackAll(result)
				return nil, err
			}
			break // DeadlineExceeded: budget spent, return what we have
		}
		remaining := messageCount - len(result)
		handles, err := n.pullLane(ctx, tier, remaining)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				nackAll(result)
				return nil, err
			}
			if ctx.Err() != nil {
				break
			}
			nackAll(result)
			return nil, err
		}
		result = append(result, handles...)
		if len(result) >= messageCount {
			break
		}
	}
	return result, nil
}

// isExpiredFetchErr reports whether err means the caller's deadline ran out
// during a lane fetch. nats.go's FetchContext validates via time.Until(deadline)
// (<= 0 -> "nats: invalid jetstream option: context deadline already exceeded",
// wrapping jetstream.ErrInvalidOption), which can race ctx.Err() in the instant
// a deadline lapses: the timer goroutine may not have set Err() yet. Both that
// error and a spent (non-canceled) context are a normal "no time left in this
// lane" outcome under the per-lane 1s priority budget, never a real failure.
// context.Canceled is deliberately NOT treated as expiry so cancellation still
// propagates (C8).
func isExpiredFetchErr(err error, ctx context.Context) bool {
	if err == nil {
		return false
	}
	if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(ctxErr, context.Canceled) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, jetstream.ErrInvalidOption) {
		return true
	}
	return strings.Contains(err.Error(), "deadline already exceeded")
}

// fetchOnce performs a single pull and folds the batch error into the returned
// error. A spent deadline means "nothing (or only a partial batch) was
// delivered" and is not an error - the caller's priority loop relies on that -
// while any other batch failure nacks whatever did arrive so the messages stay
// available. Cancellation is propagated. Errors can surface either from Fetch
// itself or from the returned batch, so both paths are handled.
func (n *NatsEngine) fetchOnce(ctx context.Context, consumer jetstream.Consumer, messageCount int) ([]common.TaskHandle, error) {
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, nil // deadline spent before the fetch even started
	}
	messages, err := consumer.Fetch(messageCount, jetstream.FetchContext(ctx))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		if isExpiredFetchErr(err, ctx) {
			return nil, nil
		}
		return nil, err
	}
	resultMessages := make([]common.TaskHandle, 0, messageCount)
	for message := range messages.Messages() {
		resultMessages = append(resultMessages, NewNatsMessageHandle(message))
	}
	if batchErr := messages.Error(); batchErr != nil {
		if errors.Is(batchErr, context.Canceled) {
			return resultMessages, batchErr
		}
		if isExpiredFetchErr(batchErr, ctx) {
			return resultMessages, nil // keep the partial batch, no spurious error
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
