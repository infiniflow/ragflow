package nats

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestTaskNackUsesConsumerBackOff(t *testing.T) {
	for _, tc := range []struct {
		name      string
		delivered uint64
		want      time.Duration
	}{
		{"first delivery", 1, 5 * time.Second},
		{"second delivery", 2, 15 * time.Second},
		{"third delivery", 3, 30 * time.Second},
		{"fourth delivery", 4, 60 * time.Second},
		{"repeat last delay", 16, 60 * time.Second},
		{"large delivery count", ^uint64(0), 60 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := &retryTestMessage{metadata: &jetstream.MsgMetadata{NumDelivered: tc.delivered}}
			schedule := []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second}
			handle := newNatsMessageHandle(msg, schedule)
			// A consumer configuration refresh must not mutate an in-flight task's policy.
			clear(schedule)
			if err := handle.Nack(); err != nil {
				t.Fatalf("Nack: %v", err)
			}
			if msg.delayedNacks != 1 || msg.delay != tc.want || msg.immediateNacks != 0 {
				t.Fatalf("Nack = (%d delayed, delay %v, %d immediate), want (1, %v, 0)", msg.delayedNacks, msg.delay, msg.immediateNacks, tc.want)
			}
		})
	}
}

func TestTaskNackWithoutBackOffPreservesImmediateRetry(t *testing.T) {
	wantErr := errors.New("send nak failed")
	msg := &retryTestMessage{ackErr: wantErr}
	if err := newNatsMessageHandle(msg, nil).Nack(); !errors.Is(err, wantErr) {
		t.Fatalf("Nack error = %v, want %v", err, wantErr)
	}
	if msg.immediateNacks != 1 || msg.delayedNacks != 0 {
		t.Fatalf("Nack = (%d immediate, %d delayed), want (1, 0)", msg.immediateNacks, msg.delayedNacks)
	}
}

func TestTaskNackMetadataFailureLeavesMessageUnsettled(t *testing.T) {
	metadataErr := errors.New("invalid ack subject")
	for _, tc := range []struct {
		name string
		msg  *retryTestMessage
	}{
		{"metadata error", &retryTestMessage{metadataErr: metadataErr}},
		{"zero delivery count", &retryTestMessage{metadata: &jetstream.MsgMetadata{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := newNatsMessageHandle(tc.msg, []time.Duration{time.Second}).Nack()
			if err == nil {
				t.Fatal("Nack succeeded with invalid delivery metadata")
			}
			if tc.msg.metadataErr != nil && !errors.Is(err, tc.msg.metadataErr) {
				t.Fatalf("Nack error = %v, want wrapped %v", err, tc.msg.metadataErr)
			}
			if tc.msg.immediateNacks != 0 || tc.msg.delayedNacks != 0 {
				t.Fatal("invalid metadata settled the message instead of leaving broker timeout recovery intact")
			}
		})
	}
}

func TestTaskNackReturnsDelayedNakError(t *testing.T) {
	wantErr := errors.New("connection closed")
	msg := &retryTestMessage{metadata: &jetstream.MsgMetadata{NumDelivered: 1}, ackErr: wantErr}
	if err := newNatsMessageHandle(msg, []time.Duration{time.Second}).Nack(); !errors.Is(err, wantErr) {
		t.Fatalf("Nack error = %v, want %v", err, wantErr)
	}
	if msg.delayedNacks != 1 || msg.immediateNacks != 0 {
		t.Fatal("delayed nak failure fell back to immediate redelivery")
	}
}

// Exercise the public task path against an embedded JetStream server. A plain
// Nak would redeliver immediately even though the consumer specifies BackOff.
// The server runs in-process with temporary storage and automatic cleanup;
// no external service is required by this default-tier test.
func TestTaskRetriesFollowExistingConsumerBackOff(t *testing.T) {
	engine := setupSyncerNATSEngine(t)
	t.Cleanup(engine.nc.Close)
	schedule := []time.Duration{150 * time.Millisecond, 350 * time.Millisecond}
	_, err := engine.stream.CreateOrUpdateConsumer(t.Context(), jetstream.ConsumerConfig{
		Name:       "RAGFLOW_CONSUMER",
		AckPolicy:  jetstream.AckExplicitPolicy,
		MaxDeliver: 6,
		MaxWaiting: 1,
		BackOff:    schedule,
	})
	if err != nil {
		t.Fatalf("create existing consumer: %v", err)
	}
	// MaxWaiting is immutable. InitConsumer falls back to the existing
	// consumer, whose real retry schedule differs from the default config.
	if err := engine.InitConsumer("tasks.>"); err != nil {
		t.Fatalf("InitConsumer: %v", err)
	}
	if got := engine.consumer.CachedInfo().Config.BackOff; !slices.Equal(got, schedule) {
		t.Fatalf("consumer BackOff = %v, want existing %v", got, schedule)
	}
	if err := engine.PublishTask("tasks.RAGFLOW", []byte(`{"task_id":"retry-task"}`)); err != nil {
		t.Fatalf("PublishTask: %v", err)
	}
	handle := fetchRetryTask(t, engine, 1)
	for i, delay := range []time.Duration{schedule[0], schedule[1], schedule[1]} {
		start := time.Now()
		if err := handle.Nack(); err != nil {
			t.Fatalf("Nack delivery %d: %v", i+1, err)
		}
		handle = fetchRetryTask(t, engine, uint64(i+2))
		if elapsed := time.Since(start); elapsed < delay*3/4 {
			t.Fatalf("delivery %d retried after %v, want BackOff %v", i+2, elapsed, delay)
		}
	}
	if err := handle.Ack(); err != nil {
		t.Fatalf("Ack recovered task: %v", err)
	}
}

func fetchRetryTask(t *testing.T, engine *NatsEngine, delivery uint64) *NatsMessageHandle {
	t.Helper()
	handles, err := engine.GetMessages(1)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("GetMessages returned %d tasks, want 1", len(handles))
	}
	handle := handles[0].(*NatsMessageHandle)
	if got := handle.GetMessage().TaskID; got != "retry-task" {
		t.Fatalf("task ID = %q, want retry-task", got)
	}
	metadata, err := handle.message.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if metadata.NumDelivered != delivery {
		t.Fatalf("NumDelivered = %d, want %d", metadata.NumDelivered, delivery)
	}
	return handle
}

type retryTestMessage struct {
	jetstream.Msg
	metadata       *jetstream.MsgMetadata
	metadataErr    error
	ackErr         error
	immediateNacks int
	delayedNacks   int
	delay          time.Duration
}

func (m *retryTestMessage) Metadata() (*jetstream.MsgMetadata, error) {
	return m.metadata, m.metadataErr
}

func (m *retryTestMessage) Nak() error {
	m.immediateNacks++
	return m.ackErr
}

func (m *retryTestMessage) NakWithDelay(delay time.Duration) error {
	m.delayedNacks++
	m.delay = delay
	return m.ackErr
}
