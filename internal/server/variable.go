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

package server

import (
	"context"
	"fmt"
	"ragflow/internal/common"
	"time"

	"go.uber.org/zap"
)

// VariableStore interface for persistent storage (e.g., Redis)
type VariableStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, exp time.Duration) bool
	SetNX(ctx context.Context, key string, value string, exp time.Duration) (bool, error)
}

const (
	// SecretKeyRedisKey is the Redis key for storing secret key
	SecretKeyRedisKey = "ragflow:system:secret_key"
	// SecretKeyTTL is the TTL for secret key in Redis (0 = no expiration)
	SecretKeyTTL = 0
)

// GetSecretKey returns the current secret key
func GetSecretKey(ctx context.Context, store VariableStore) (string, error) {
	if globalConfig.GetSecretKey() != "" {
		return globalConfig.GetSecretKey(), nil
	}

	generatedKey, err := common.GenerateSecretKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate secret key: %w", err)
	}

	secretKey, err := GetOrCreateKey(ctx, store, SecretKeyRedisKey, generatedKey)
	if err != nil {
		return "", fmt.Errorf("failed to get secret key: %w", err)
	}
	return secretKey, nil
}

// GetOrCreateKey gets a key from store, or creates it if not exists
// - If key exists in store, returns the stored value
// - If key doesn't exist, calls createFn to generate value, stores it, and returns it
// - Uses SetNX to ensure atomic creation (only one caller succeeds when key doesn't exist)
func GetOrCreateKey(ctx context.Context, store VariableStore, key string, newValue string) (string, error) {
	if store == nil {
		err := fmt.Errorf("store is nil")
		common.Warn("VariableStore is nil, cannot get or create key", zap.String("key", key))
		return "store is nil", err
	}

	// Try to get existing value
	value, err := store.Get(ctx, key)
	if err != nil {
		common.Warn("Failed to get key from store", zap.String("key", key), zap.Error(err))
		return "", err
	}

	// Key exists, return the value
	if value != "" {
		common.Debug("Key found in store", zap.String("key", key))
		return value, nil
	}

	// Key doesn't exist, generate new value
	common.Info("Generating new value for key", zap.String("key", key))

	// Try to set with NX (only if not exists) - ensures atomicity
	created, err := store.SetNX(ctx, key, newValue, SecretKeyTTL)
	if err != nil {
		return "", fmt.Errorf("failed to create key %q: %w", key, err)
	}
	if created {
		common.Info("New value stored successfully", zap.String("key", key))
		return newValue, nil
	}

	// Another process might have set it, try to get again
	value, err = store.Get(ctx, key)
	if err != nil {
		common.Warn("Failed to get key after SetNX", zap.String("key", key), zap.Error(err))
		return newValue, nil // Return our generated value as fallback
	}

	if value != "" {
		common.Info("Using value set by another process", zap.String("key", key))
		return value, nil
	}

	// If still empty, use our generated value
	return newValue, nil
}
