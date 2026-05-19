package cache

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"ragflow/internal/common"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Message represents a message in the queue.
type Message struct {
	ID         string                 `json:"id"`
	Topic      string                 `json:"topic"`
	Payload    map[string]interface{} `json:"payload"`
	RetryCount int                    `json:"retry_count"`
	CreatedAt  time.Time              `json:"created_at"`
}

// Producer represents a message producer.
type Producer struct {
	client *RedisClient // Uses cache.RedisClient directly
	topic  string
}

// Consumer represents a message consumer.
type Consumer struct {
	client       *RedisClient
	topic        string
	groupName    string
	consumerName string
	handler      HandlerFunc
	stopCh       chan struct{}
	stopOnce     sync.Once // Ensures stopCh is closed only once
	wg           sync.WaitGroup
	config       ConsumerConfig
}

// HandlerFunc is the message processing function signature.
type HandlerFunc func(msg *Message) error

// ConsumerConfig contains configuration options for the consumer.
type ConsumerConfig struct {
	BatchSize        int           // Number of messages to consume in batch, default 1
	BlockTimeout     time.Duration // Blocking timeout, default 5 seconds
	MaxRetries       int           // Maximum retry attempts, default 3
	RetryDelay       time.Duration // Delay between retries, default 1 second
	Concurrent       int           // Number of concurrent workers, default 1
	EnableDeadLetter bool          // Whether to enable dead letter queue
	DeadLetterTopic  string        // Dead letter queue topic
}

// ProducerConfig contains configuration options for the producer.
type ProducerConfig struct {
	MaxLen int64 // Maximum stream length
	Approx bool  // Whether to use approximate trimming
}

// DeadLetterMessage represents a message sent to the dead letter queue.
type DeadLetterMessage struct {
	OriginalMsg *Message  `json:"original_msg"`
	Error       string    `json:"error"`
	FailedAt    time.Time `json:"failed_at"`
}

// NewProducer creates a new message producer.
func NewProducer(topic string, config *ProducerConfig) (*Producer, error) {
	client := Get()
	if client == nil || !IsEnabled() {
		return nil, fmt.Errorf("redis client not available")
	}

	if config == nil {
		config = &ProducerConfig{
			MaxLen: 1024 * 1024,
			Approx: true,
		}
	}

	return &Producer{
		client: client,
		topic:  topic,
	}, nil
}

// Send sends a single message to the queue.
func (p *Producer) Send(payload map[string]interface{}) (string, error) {
	msg := &Message{
		ID:         generateMsgID(),
		Topic:      p.topic,
		Payload:    payload,
		RetryCount: 0,
		CreatedAt:  time.Now(),
	}

	// Serialize the message
	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("marshal message failed: %w", err)
	}

	// Use cache.RedisClient's QueueProduct method
	success := p.client.QueueProduct(p.topic, string(data))
	if !success {
		return "", fmt.Errorf("failed to send message to queue: %s", p.topic)
	}

	return msg.ID, nil
}

// SendWithRetry sends a message with retry logic.
func (p *Producer) SendWithRetry(payload map[string]interface{}, maxRetries int) (string, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		msgID, err := p.Send(payload)
		if err == nil {
			return msgID, nil
		}
		lastErr = err
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	return "", fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// SendBatch sends multiple messages in batch.
func (p *Producer) SendBatch(payloads []map[string]interface{}) ([]string, []error) {
	ids := make([]string, 0, len(payloads))
	errors := make([]error, 0)

	for _, payload := range payloads {
		id, err := p.Send(payload)
		if err != nil {
			errors = append(errors, fmt.Errorf("payload %v failed: %w", payload, err))
			continue
		}
		ids = append(ids, id)
	}

	return ids, errors
}

// NewConsumer creates a new message consumer using the global Redis client.
func NewConsumer(topic, groupName, consumerName string, handler HandlerFunc, config *ConsumerConfig) (*Consumer, error) {
	client := Get()
	if client == nil || !IsEnabled() {
		return nil, fmt.Errorf("redis client not available")
	}

	if config == nil {
		config = &ConsumerConfig{
			BatchSize:        1,
			BlockTimeout:     5 * time.Second,
			MaxRetries:       3,
			RetryDelay:       1 * time.Second,
			Concurrent:       1,
			EnableDeadLetter: true,
			DeadLetterTopic:  fmt.Sprintf("%s:deadletter", topic),
		}
	}

	return &Consumer{
		client:       client,
		topic:        topic,
		groupName:    groupName,
		consumerName: consumerName,
		handler:      handler,
		stopCh:       make(chan struct{}),
		config:       *config,
	}, nil
}

// Start starts the consumer with concurrent workers.
func (c *Consumer) Start() error {
	for i := 0; i < c.config.Concurrent; i++ {
		c.wg.Add(1)
		go c.consumeLoop(i)
	}

	common.Info("Consumer started",
		zap.String("topic", c.topic),
		zap.String("group", c.groupName),
		zap.String("consumer", c.consumerName),
		zap.Int("concurrent", c.config.Concurrent),
	)

	return nil
}

// Stop gracefully stops the consumer.
// Safe to call multiple times due to sync.Once protection.
func (c *Consumer) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
	c.wg.Wait()
	common.Info("Consumer stopped", zap.String("topic", c.topic))
}

// consumeLoop is the main consumption loop for a worker.
func (c *Consumer) consumeLoop(workerID int) {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		default:
			c.consumeOne(workerID)
		}
	}
}

// consumeOne consumes a single message.
func (c *Consumer) consumeOne(workerID int) {
	// Use cache.RedisClient's QueueConsumer method
	redisMsg, err := c.client.QueueConsumer(c.topic, c.groupName, c.consumerName, "")
	if err != nil {
		common.Warn("Failed to consume message",
			zap.Error(err),
			zap.Int("worker", workerID),
		)
		time.Sleep(100 * time.Millisecond)
		return
	}

	if redisMsg == nil {
		// No message available, brief wait
		time.Sleep(10 * time.Millisecond)
		return
	}

	// Parse the message
	msg, err := c.parseMessage(redisMsg)
	if err != nil {
		common.Warn("Failed to parse message",
			zap.Error(err),
			zap.String("msgID", redisMsg.GetMsgID()),
		)
		// Parse failure: ACK to avoid blocking the queue
		redisMsg.Ack()
		return
	}

	// Process the message
	err = c.handler(msg)
	if err == nil {
		// Success: ACK the message
		redisMsg.Ack()
		common.Debug("Message processed successfully",
			zap.String("msgID", msg.ID),
			zap.Int("worker", workerID),
		)
		return
	}

	// Handle processing failure
	c.handleFailure(redisMsg, msg, err, workerID)
}

// parseMessage parses a Redis message into a Message struct.
func (c *Consumer) parseMessage(redisMsg *RedisMsg) (*Message, error) {
	msgData := redisMsg.GetMessage()
	if msgData == nil {
		return nil, fmt.Errorf("empty message data")
	}

	// Extract the "message" field from the map
	msgStr, ok := msgData["message"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid message format")
	}

	var msg Message
	if err := json.Unmarshal([]byte(msgStr), &msg); err != nil {
		return nil, fmt.Errorf("unmarshal message failed: %w", err)
	}

	return &msg, nil
}

// handleFailure handles message processing failures with retry logic.
// Messages are only ACKed after successful requeue or when max retries are exceeded.
// If max retries are exceeded, the message is sent to the dead letter queue (if enabled)
// and then ACKed to prevent poison messages from blocking the queue.
func (c *Consumer) handleFailure(redisMsg *RedisMsg, msg *Message, handlerErr error, workerID int) {
	msg.RetryCount++

	common.Warn("Message processing failed",
		zap.String("msgID", msg.ID),
		zap.Int("retryCount", msg.RetryCount),
		zap.Int("maxRetries", c.config.MaxRetries),
		zap.Error(handlerErr),
		zap.Int("worker", workerID),
	)

	if msg.RetryCount >= c.config.MaxRetries {
		// Max retries exceeded: send to dead letter queue if enabled
		if c.config.EnableDeadLetter {
			c.sendToDeadLetter(msg, handlerErr)
		}
		// ACK the original message to prevent poison pill from blocking the queue
		// Note: This may result in message loss if dead letter queue is disabled or fails
		redisMsg.Ack()
		return
	}

	// Retry delay
	if c.config.RetryDelay > 0 {
		time.Sleep(c.config.RetryDelay)
	}

	// Attempt to requeue the message
	if c.requeueMessage(msg) {
		// Requeue successful: ACK the original message
		redisMsg.Ack()
	} else {
		// Requeue failed: do NOT ACK, message remains in pending list for redelivery
		common.Warn("Message requeue failed, message remains pending",
			zap.String("msgID", msg.ID),
		)
	}
}

// requeueMessage requeues a message for retry.
// Returns true if successful, false otherwise.
func (c *Consumer) requeueMessage(msg *Message) bool {
	data, err := json.Marshal(msg)
	if err != nil {
		common.Error("Failed to marshal message for requeue", fmt.Errorf("msgID: %s, err: %w", msg.ID, err))
		return false
	}

	// Use cache.RedisClient's QueueProduct method
	success := c.client.QueueProduct(c.topic, string(data))
	if !success {
		common.Error("Failed to requeue message", fmt.Errorf("msgID: %s", msg.ID))
		return false
	}
	return true
}

// sendToDeadLetter sends a failed message to the dead letter queue.
func (c *Consumer) sendToDeadLetter(msg *Message, handlerErr error) {
	deadMsg := &DeadLetterMessage{
		OriginalMsg: msg,
		Error:       handlerErr.Error(),
		FailedAt:    time.Now(),
	}

	data, err := json.Marshal(deadMsg)
	if err != nil {
		common.Error("Failed to marshal dead letter message", fmt.Errorf("msgID: %s, err: %w", msg.ID, err))
		return
	}

	// Use cache.RedisClient's QueueProduct method
	success := c.client.QueueProduct(c.config.DeadLetterTopic, string(data))
	if !success {
		common.Error("Failed to send to dead letter queue", fmt.Errorf("msgID: %s, deadTopic: %s", msg.ID, c.config.DeadLetterTopic))
		return
	}

	common.Warn("Message sent to dead letter queue",
		zap.String("msgID", msg.ID),
		zap.String("deadTopic", c.config.DeadLetterTopic),
		zap.Error(handlerErr),
	)
}

// GetPendingCount returns the number of pending messages in the queue.
func (c *Consumer) GetPendingCount() (int64, error) {
	pendingMsgs, err := c.client.GetPendingMsg(c.topic, c.groupName)
	if err != nil {
		return 0, fmt.Errorf("get pending messages failed: %w", err)
	}

	var total int64
	for _, p := range pendingMsgs {
		total += p.RetryCount
	}
	return total, nil
}

// GetQueueInfo returns information about the queue.
func (c *Consumer) GetQueueInfo() (map[string]interface{}, error) {
	return c.client.QueueInfo(c.topic, c.groupName)
}

// RequeuePending requeues a specific pending message by ID.
// Note: This function currently does not return errors from the underlying client.
func (c *Consumer) RequeuePending(msgID string) {
	c.client.RequeueMsg(c.topic, c.groupName, msgID)
}

// generateMsgID generates a unique message ID.
func generateMsgID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.New().String()[:8])
}
