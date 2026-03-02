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

package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"ragflow/internal/server"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"ragflow/internal/logger"
)

var (
	globalClient *RedisClient
	once         sync.Once
)

// RedisClient wraps go-redis client with additional utility methods
type RedisClient struct {
	client           *redis.Client
	luaDeleteIfEqual *redis.Script
	luaTokenBucket   *redis.Script
	luaAutoIncrement *redis.Script
	config           *server.RedisConfig
}

// RedisMsg represents a message from Redis Stream
type RedisMsg struct {
	consumer  *redis.Client
	queueName string
	groupName string
	msgID     string
	message   map[string]interface{}
}

// Lua scripts
const (
	luaDeleteIfEqualScript = `
		local current_value = redis.call('get', KEYS[1])
		if current_value and current_value == ARGV[1] then
			redis.call('del', KEYS[1])
			return 1
		end
		return 0
	`

	luaTokenBucketScript = `
		local key       = KEYS[1]
		local capacity  = tonumber(ARGV[1])
		local rate      = tonumber(ARGV[2])
		local now       = tonumber(ARGV[3])
		local cost      = tonumber(ARGV[4])

		local data = redis.call("HMGET", key, "tokens", "timestamp")
		local tokens = tonumber(data[1])
		local last_ts = tonumber(data[2])

		if tokens == nil then
			tokens = capacity
			last_ts = now
		end

		local delta = math.max(0, now - last_ts)
		tokens = math.min(capacity, tokens + delta * rate)

		if tokens < cost then
			return {0, tokens}
		end

		tokens = tokens - cost

		redis.call("HMSET", key,
			"tokens", tokens,
			"timestamp", now
		)

		redis.call("EXPIRE", key, math.ceil(capacity / rate * 2))

		return {1, tokens}
	`
)

// Init initializes Redis client
func Init(cfg *server.RedisConfig) error {
	var initErr error
	once.Do(func() {
		if cfg.Host == "" {
			logger.Info("Redis host not configured, skipping Redis initialization")
			return
		}

		client := redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Password: cfg.Password,
			DB:       cfg.DB,
		})

		// Test connection
		ctx, cancel := context.WithTimeout(context.Background(), server.DefaultConnectTimeout)
		defer cancel()

		if err := client.Ping(ctx).Err(); err != nil {
			initErr = fmt.Errorf("failed to connect to Redis: %w", err)
			return
		}

		globalClient = &RedisClient{
			client:           client,
			config:           cfg,
			luaDeleteIfEqual: redis.NewScript(luaDeleteIfEqualScript),
			luaTokenBucket:   redis.NewScript(luaTokenBucketScript),
		}

		logger.Info("Redis client initialized",
			zap.String("host", cfg.Host),
			zap.Int("port", cfg.Port),
			zap.Int("db", cfg.DB),
		)
	})
	return initErr
}

// Get gets global Redis client instance
func Get() *RedisClient {
	return globalClient
}

// Close closes Redis client
func Close() error {
	if globalClient != nil && globalClient.client != nil {
		return globalClient.client.Close()
	}
	return nil
}

// IsEnabled checks if Redis is enabled (configured and initialized)
func IsEnabled() bool {
	return globalClient != nil && globalClient.client != nil
}

// Health checks if Redis is healthy
func (r *RedisClient) Health() bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	if err := r.client.Ping(ctx).Err(); err != nil {
		return false
	}

	testKey := "health_check_" + uuid.New().String()
	testValue := "yy"
	if err := r.client.Set(ctx, testKey, testValue, 3*time.Second).Err(); err != nil {
		return false
	}

	val, err := r.client.Get(ctx, testKey).Result()
	if err != nil || val != testValue {
		return false
	}
	return true
}

// Info returns Redis server information
func (r *RedisClient) Info() map[string]interface{} {
	if r.client == nil {
		return nil
	}
	ctx := context.Background()
	infoStr, err := r.client.Info(ctx).Result()
	if err != nil {
		logger.Warn("Failed to get Redis info", zap.Error(err))
		return nil
	}

	// Parse info string to map
	info := make(map[string]string)
	lines := splitLines(infoStr)
	for _, line := range lines {
		if line == "" || line[0] == '#' {
			continue
		}
		parts := splitN(line, ":", 2)
		if len(parts) == 2 {
			info[parts[0]] = parts[1]
		}
	}

	result := map[string]interface{}{
		"redis_version":             info["redis_version"],
		"server_mode":               getServerMode(info),
		"used_memory":               info["used_memory_human"],
		"total_system_memory":       info["total_system_memory_human"],
		"mem_fragmentation_ratio":   info["mem_fragmentation_ratio"],
		"connected_clients":         parseInt(info["connected_clients"]),
		"blocked_clients":           parseInt(info["blocked_clients"]),
		"instantaneous_ops_per_sec": parseInt(info["instantaneous_ops_per_sec"]),
		"total_commands_processed":  parseInt(info["total_commands_processed"]),
	}
	return result
}

func getServerMode(info map[string]string) string {
	if mode, ok := info["server_mode"]; ok {
		return mode
	}
	return info["redis_mode"]
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitN(s, sep string, n int) []string {
	if n <= 0 {
		return []string{s}
	}
	idx := -1
	for i := 0; i < len(s)-len(sep)+1; i++ {
		if s[i:i+len(sep)] == sep {
			idx = i
			break
		}
	}
	if idx == -1 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+len(sep):]}
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

// IsAlive checks if Redis client is alive
func (r *RedisClient) IsAlive() bool {
	return r.client != nil
}

// Exist checks if key exists
func (r *RedisClient) Exist(key string) (bool, error) {
	if r.client == nil {
		return false, nil
	}
	ctx := context.Background()
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		logger.Warn("Redis Exist error", zap.String("key", key), zap.Error(err))
		return false, err
	}
	return exists > 0, nil
}

// Get gets value by key
func (r *RedisClient) Get(key string) (string, error) {
	if r.client == nil {
		return "", nil
	}
	ctx := context.Background()
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		logger.Warn("Redis Get error", zap.String("key", key), zap.Error(err))
		return "", err
	}
	return val, nil
}

// SetObj sets object with JSON serialization
func (r *RedisClient) SetObj(key string, obj interface{}, exp time.Duration) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	data, err := json.Marshal(obj)
	if err != nil {
		logger.Warn("Redis SetObj marshal error", zap.String("key", key), zap.Error(err))
		return false
	}
	if err := r.client.Set(ctx, key, data, exp).Err(); err != nil {
		logger.Warn("Redis SetObj error", zap.String("key", key), zap.Error(err))
		return false
	}
	return true
}

// GetObj gets and unmarshals object from Redis
func (r *RedisClient) GetObj(key string, dest interface{}) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	data, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false
	}
	if err != nil {
		logger.Warn("Redis GetObj error", zap.String("key", key), zap.Error(err))
		return false
	}
	if err := json.Unmarshal([]byte(data), dest); err != nil {
		logger.Warn("Redis GetObj unmarshal error", zap.String("key", key), zap.Error(err))
		return false
	}
	return true
}

// Set sets value with expiration
func (r *RedisClient) Set(key string, value string, exp time.Duration) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	if err := r.client.Set(ctx, key, value, exp).Err(); err != nil {
		logger.Warn("Redis Set error", zap.String("key", key), zap.Error(err))
		return false
	}
	return true
}

// SetNX sets value only if key does not exist
func (r *RedisClient) SetNX(key string, value string, exp time.Duration) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	ok, err := r.client.SetNX(ctx, key, value, exp).Result()
	if err != nil {
		logger.Warn("Redis SetNX error", zap.String("key", key), zap.Error(err))
		return false
	}
	return ok
}

// GetOrCreateSecretKey atomically retrieves an existing key or creates a new one
// Uses Redis SETNX command to ensure atomicity across multiple goroutines/processes
func (r *RedisClient) GetOrCreateKey(key string, value string) (string, error) {
	if r.client == nil {
		return "", nil
	}
	ctx := context.Background()
	// First, try to get the existing key
	existingKey, err := r.client.Get(ctx, key).Result()
	if err == nil {
		logger.Warn("Redis Get error", zap.String("key", key), zap.Error(err))
		// Successfully retrieved existing key
		return existingKey, nil
	}

	// Use SETNX to atomically set the key only if it doesn't exist
	// SETNX returns true if the key was set, false if it already existed
	success, err := r.client.SetNX(ctx, key, value, 0).Result()
	if err != nil {
		return "", fmt.Errorf("failed to set key in Redis: %v", err)
	}

	if success {
		// This goroutine successfully set the key
		return value, nil
	}

	// SETNX failed, meaning another goroutine set the key concurrently
	// Retrieve and return that key
	finalKey, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("failed to get key set by another process: %v", err)
	}

	return finalKey, nil
}

// SAdd adds member to set
func (r *RedisClient) SAdd(key string, member string) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	if err := r.client.SAdd(ctx, key, member).Err(); err != nil {
		logger.Warn("Redis SAdd error", zap.String("key", key), zap.Error(err))
		return false
	}
	return true
}

// SRem removes member from set
func (r *RedisClient) SRem(key string, member string) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	if err := r.client.SRem(ctx, key, member).Err(); err != nil {
		logger.Warn("Redis SRem error", zap.String("key", key), zap.Error(err))
		return false
	}
	return true
}

// SMembers returns all members of a set
func (r *RedisClient) SMembers(key string) ([]string, error) {
	if r.client == nil {
		return nil, nil
	}
	ctx := context.Background()
	members, err := r.client.SMembers(ctx, key).Result()
	if err != nil {
		logger.Warn("Redis SMembers error", zap.String("key", key), zap.Error(err))
		return nil, err
	}
	return members, nil
}

// SIsMember checks if member exists in set
func (r *RedisClient) SIsMember(key string, member string) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	ok, err := r.client.SIsMember(ctx, key, member).Result()
	if err != nil {
		logger.Warn("Redis SIsMember error", zap.String("key", key), zap.Error(err))
		return false
	}
	return ok
}

// ZAdd adds member with score to sorted set
func (r *RedisClient) ZAdd(key string, member string, score float64) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	if err := r.client.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err(); err != nil {
		logger.Warn("Redis ZAdd error", zap.String("key", key), zap.Error(err))
		return false
	}
	return true
}

// ZCount returns count of members with score in range
func (r *RedisClient) ZCount(key string, min, max float64) int64 {
	if r.client == nil {
		return 0
	}
	ctx := context.Background()
	count, err := r.client.ZCount(ctx, key, fmt.Sprintf("%f", min), fmt.Sprintf("%f", max)).Result()
	if err != nil {
		logger.Warn("Redis ZCount error", zap.String("key", key), zap.Error(err))
		return 0
	}
	return count
}

// ZPopMin pops minimum score members from sorted set
func (r *RedisClient) ZPopMin(key string, count int) ([]redis.Z, error) {
	if r.client == nil {
		return nil, nil
	}
	ctx := context.Background()
	members, err := r.client.ZPopMin(ctx, key, int64(count)).Result()
	if err != nil {
		logger.Warn("Redis ZPopMin error", zap.String("key", key), zap.Error(err))
		return nil, err
	}
	return members, nil
}

// ZRangeByScore returns members with score in range
func (r *RedisClient) ZRangeByScore(key string, min, max float64) ([]string, error) {
	if r.client == nil {
		return nil, nil
	}
	ctx := context.Background()
	members, err := r.client.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%f", min),
		Max: fmt.Sprintf("%f", max),
	}).Result()
	if err != nil {
		logger.Warn("Redis ZRangeByScore error", zap.String("key", key), zap.Error(err))
		return nil, err
	}
	return members, nil
}

// ZRemRangeByScore removes members with score in range
func (r *RedisClient) ZRemRangeByScore(key string, min, max float64) int64 {
	if r.client == nil {
		return 0
	}
	ctx := context.Background()
	count, err := r.client.ZRemRangeByScore(ctx, key, fmt.Sprintf("%f", min), fmt.Sprintf("%f", max)).Result()
	if err != nil {
		logger.Warn("Redis ZRemRangeByScore error", zap.String("key", key), zap.Error(err))
		return 0
	}
	return count
}

// IncrBy increments key by increment
func (r *RedisClient) IncrBy(key string, increment int64) (int64, error) {
	if r.client == nil {
		return 0, nil
	}
	ctx := context.Background()
	val, err := r.client.IncrBy(ctx, key, increment).Result()
	if err != nil {
		logger.Warn("Redis IncrBy error", zap.String("key", key), zap.Error(err))
		return 0, err
	}
	return val, nil
}

// DecrBy decrements key by decrement
func (r *RedisClient) DecrBy(key string, decrement int64) (int64, error) {
	if r.client == nil {
		return 0, nil
	}
	ctx := context.Background()
	val, err := r.client.DecrBy(ctx, key, decrement).Result()
	if err != nil {
		logger.Warn("Redis DecrBy error", zap.String("key", key), zap.Error(err))
		return 0, err
	}
	return val, nil
}

// GenerateAutoIncrementID generates auto-increment ID
func (r *RedisClient) GenerateAutoIncrementID(keyPrefix string, namespace string, increment int64, ensureMinimum *int64) int64 {
	if r.client == nil {
		return -1
	}
	if keyPrefix == "" {
		keyPrefix = "id_generator"
	}
	if namespace == "" {
		namespace = "default"
	}
	if increment == 0 {
		increment = 1
	}

	redisKey := fmt.Sprintf("%s:%s", keyPrefix, namespace)
	ctx := context.Background()

	// Check if key exists
	exists, err := r.client.Exists(ctx, redisKey).Result()
	if err != nil {
		logger.Warn("Redis GenerateAutoIncrementID error", zap.Error(err))
		return -1
	}

	if exists == 0 && ensureMinimum != nil {
		startID := int64(math.Max(1, float64(*ensureMinimum)))
		r.client.Set(ctx, redisKey, startID, 0)
		return startID
	}

	// Get current value
	if ensureMinimum != nil {
		current, err := r.client.Get(ctx, redisKey).Int64()
		if err == nil && current < *ensureMinimum {
			r.client.Set(ctx, redisKey, *ensureMinimum, 0)
			return *ensureMinimum
		}
	}

	// Increment
	nextID, err := r.client.IncrBy(ctx, redisKey, increment).Result()
	if err != nil {
		logger.Warn("Redis GenerateAutoIncrementID increment error", zap.Error(err))
		return -1
	}

	return nextID
}

// Transaction sets key with NX flag (transaction-like behavior)
func (r *RedisClient) Transaction(key string, value string, exp time.Duration) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	pipe := r.client.Pipeline()
	pipe.SetNX(ctx, key, value, exp)
	_, err := pipe.Exec(ctx)
	if err != nil {
		logger.Warn("Redis Transaction error", zap.String("key", key), zap.Error(err))
		return false
	}
	return true
}

// QueueProduct produces a message to Redis Stream
func (r *RedisClient) QueueProduct(queue string, message interface{}) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		data, err := json.Marshal(message)
		if err != nil {
			logger.Warn("Redis QueueProduct marshal error", zap.Error(err))
			return false
		}

		_, err = r.client.XAdd(ctx, &redis.XAddArgs{
			Stream: queue,
			Values: map[string]interface{}{"message": string(data)},
		}).Result()
		if err == nil {
			return true
		}
		logger.Warn("Redis QueueProduct error", zap.String("queue", queue), zap.Error(err))
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// QueueConsumer consumes a message from Redis Stream
func (r *RedisClient) QueueConsumer(queueName, groupName, consumerName string, msgID string) (*RedisMsg, error) {
	if r.client == nil {
		return nil, nil
	}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		// Create consumer group if not exists
		groups, err := r.client.XInfoGroups(ctx, queueName).Result()
		if err != nil && err.Error() != "no such key" {
			logger.Warn("Redis QueueConsumer XInfoGroups error", zap.Error(err))
		}

		groupExists := false
		for _, g := range groups {
			if g.Name == groupName {
				groupExists = true
				break
			}
		}

		if !groupExists {
			err = r.client.XGroupCreateMkStream(ctx, queueName, groupName, "0").Err()
			if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
				logger.Warn("Redis QueueConsumer XGroupCreate error", zap.Error(err))
			}
		}

		if msgID == "" {
			msgID = ">"
		}

		messages, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    groupName,
			Consumer: consumerName,
			Streams:  []string{queueName, msgID},
			Count:    1,
			Block:    5 * time.Second,
		}).Result()

		if err == redis.Nil {
			return nil, nil
		}
		if err != nil {
			logger.Warn("Redis QueueConsumer XReadGroup error", zap.Error(err))
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if len(messages) == 0 || len(messages[0].Messages) == 0 {
			return nil, nil
		}

		msg := messages[0].Messages[0]
		var messageData map[string]interface{}
		if msgStr, ok := msg.Values["message"].(string); ok {
			json.Unmarshal([]byte(msgStr), &messageData)
		}

		return &RedisMsg{
			consumer:  r.client,
			queueName: queueName,
			groupName: groupName,
			msgID:     msg.ID,
			message:   messageData,
		}, nil
	}
	return nil, nil
}

// Ack acknowledges the message
func (m *RedisMsg) Ack() bool {
	if m.consumer == nil {
		return false
	}
	ctx := context.Background()
	err := m.consumer.XAck(ctx, m.queueName, m.groupName, m.msgID).Err()
	if err != nil {
		logger.Warn("RedisMsg Ack error", zap.Error(err))
		return false
	}
	return true
}

// GetMessage returns the message data
func (m *RedisMsg) GetMessage() map[string]interface{} {
	return m.message
}

// GetMsgID returns the message ID
func (m *RedisMsg) GetMsgID() string {
	return m.msgID
}

// GetPendingMsg gets pending messages
func (r *RedisClient) GetPendingMsg(queue, groupName string) ([]redis.XPendingExt, error) {
	if r.client == nil {
		return nil, nil
	}
	ctx := context.Background()
	msgs, err := r.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: queue,
		Group:  groupName,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if err != nil {
		if err.Error() != "No such key" {
			logger.Warn("Redis GetPendingMsg error", zap.Error(err))
		}
		return nil, err
	}
	return msgs, nil
}

// RequeueMsg requeues a message
func (r *RedisClient) RequeueMsg(queue, groupName, msgID string) {
	if r.client == nil {
		return
	}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		msgs, err := r.client.XRange(ctx, queue, msgID, msgID).Result()
		if err != nil {
			logger.Warn("Redis RequeueMsg XRange error", zap.Error(err))
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if len(msgs) == 0 {
			return
		}

		r.client.XAdd(ctx, &redis.XAddArgs{
			Stream: queue,
			Values: msgs[0].Values,
		})
		r.client.XAck(ctx, queue, groupName, msgID)
		return
	}
}

// QueueInfo returns queue group info
func (r *RedisClient) QueueInfo(queue, groupName string) (map[string]interface{}, error) {
	if r.client == nil {
		return nil, nil
	}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		groups, err := r.client.XInfoGroups(ctx, queue).Result()
		if err != nil {
			logger.Warn("Redis QueueInfo error", zap.Error(err))
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for _, g := range groups {
			if g.Name == groupName {
				return map[string]interface{}{
					"name":           g.Name,
					"consumers":      g.Consumers,
					"pending":        g.Pending,
					"last_delivered": g.LastDeliveredID,
				}, nil
			}
		}
		return nil, nil
	}
	return nil, nil
}

// DeleteIfEqual deletes key if its value equals expected value (atomic)
func (r *RedisClient) DeleteIfEqual(key, expectedValue string) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	result, err := r.luaDeleteIfEqual.Run(ctx, r.client, []string{key}, expectedValue).Result()
	if err != nil {
		logger.Warn("Redis DeleteIfEqual error", zap.Error(err))
		return false
	}
	return result.(int64) == 1
}

// Delete deletes a key
func (r *RedisClient) Delete(key string) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	if err := r.client.Del(ctx, key).Err(); err != nil {
		logger.Warn("Redis Delete error", zap.String("key", key), zap.Error(err))
		return false
	}
	return true
}

// Expire sets expiration on a key
func (r *RedisClient) Expire(key string, exp time.Duration) bool {
	if r.client == nil {
		return false
	}
	ctx := context.Background()
	if err := r.client.Expire(ctx, key, exp).Err(); err != nil {
		logger.Warn("Redis Expire error", zap.String("key", key), zap.Error(err))
		return false
	}
	return true
}

// TTL gets remaining time to live of a key
func (r *RedisClient) TTL(key string) time.Duration {
	if r.client == nil {
		return -2
	}
	ctx := context.Background()
	ttl, err := r.client.TTL(ctx, key).Result()
	if err != nil {
		logger.Warn("Redis TTL error", zap.String("key", key), zap.Error(err))
		return -2
	}
	return ttl
}

// DistributedLock distributed lock implementation
type DistributedLock struct {
	client          *RedisClient
	lockKey         string
	lockValue       string
	timeout         time.Duration
	blockingTimeout time.Duration
}

// NewDistributedLock creates a new distributed lock
func NewDistributedLock(lockKey string, lockValue string, timeout time.Duration, blockingTimeout time.Duration) *DistributedLock {
	if globalClient == nil {
		return nil
	}
	if lockValue == "" {
		lockValue = uuid.New().String()
	}
	return &DistributedLock{
		client:          globalClient,
		lockKey:         lockKey,
		lockValue:       lockValue,
		timeout:         timeout,
		blockingTimeout: blockingTimeout,
	}
}

// Acquire acquires the lock
func (l *DistributedLock) Acquire() bool {
	if l.client == nil {
		return false
	}
	// Delete if stale
	l.client.DeleteIfEqual(l.lockKey, l.lockValue)
	return l.client.SetNX(l.lockKey, l.lockValue, l.timeout)
}

// SpinAcquire keeps trying to acquire the lock
func (l *DistributedLock) SpinAcquire(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			l.client.DeleteIfEqual(l.lockKey, l.lockValue)
			if l.client.SetNX(l.lockKey, l.lockValue, l.timeout) {
				return nil
			}
			time.Sleep(10 * time.Second)
		}
	}
}

// Release releases the lock
func (l *DistributedLock) Release() bool {
	if l.client == nil {
		return false
	}
	return l.client.DeleteIfEqual(l.lockKey, l.lockValue)
}

// TokenBucket token bucket rate limiter
type TokenBucket struct {
	client   *RedisClient
	key      string
	capacity float64
	rate     float64
}

// NewTokenBucket creates a new token bucket
func NewTokenBucket(key string, capacity, rate float64) *TokenBucket {
	if globalClient == nil {
		return nil
	}
	return &TokenBucket{
		client:   globalClient,
		key:      key,
		capacity: capacity,
		rate:     rate,
	}
}

// Allow checks if request is allowed
func (tb *TokenBucket) Allow(cost float64) (bool, float64) {
	if tb.client == nil || tb.client.client == nil {
		return true, 0
	}
	ctx := context.Background()
	now := float64(time.Now().Unix())

	result, err := tb.client.luaTokenBucket.Run(ctx, tb.client.client, []string{tb.key},
		tb.capacity, tb.rate, now, cost).Result()
	if err != nil {
		logger.Warn("TokenBucket Allow error", zap.Error(err))
		return true, 0
	}

	values := result.([]interface{})
	allowed := values[0].(int64) == 1
	tokens := values[1].(int64)
	return allowed, float64(tokens)
}

// GetClient returns the underlying go-redis client for advanced usage
func (r *RedisClient) GetClient() *redis.Client {
	return r.client
}

// RandomSleep sleeps for random duration between min and max milliseconds
func RandomSleep(minMs, maxMs int) {
	duration := time.Duration(rand.Intn(maxMs-minMs)+minMs) * time.Millisecond
	time.Sleep(duration)
}
