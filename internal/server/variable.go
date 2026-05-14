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
	"ragflow/internal/utility"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Variables holds all runtime variables that can be changed during system operation
// Unlike Config, these can be modified at runtime
type Variables struct {
	//SecretKey string `json:"secret_key"`
}

// VariableStore interface for persistent storage (e.g., Redis)
type VariableStore interface {
	Get(key string) (string, error)
	Set(key string, value string, exp time.Duration) bool
	SetNX(key string, value string, exp time.Duration) bool
}

var (
	globalVariables *Variables
	variablesOnce   sync.Once
	variablesMu     sync.RWMutex
)

const (
	// DefaultSecretKey is used when no secret key is found in storage
	DefaultSecretKey = "infiniflow-token"
	// SecretKeyRedisKey is the Redis key for storing secret key
	SecretKeyRedisKey = "ragflow:system:secret_key"
	// SecretKeyTTL is the TTL for secret key in Redis (0 = no expiration)
	SecretKeyTTL = 0
)

// InitVariables initializes all runtime variables from persistent storage
// This should be called after Config and Cache are initialized
func InitVariables(store VariableStore) error {
	var initErr error
	variablesOnce.Do(func() {
		globalVariables = &Variables{}

		//// secret key
		//generatedKey, err := utility.GenerateSecretKey()
		//if err != nil {
		//	initErr = fmt.Errorf("failed to generate secret key: %w", err)
		//}
		//
		//// Initialize SecretKey
		//secretKey, err := GetOrCreateKey(store, SecretKeyRedisKey, generatedKey)
		//if err != nil {
		//	initErr = fmt.Errorf("failed to initialize secret key: %w", err)
		//} else {
		//	globalVariables.SecretKey = secretKey
		//	common.Info("Secret key initialized from store")
		//}

		common.Info("Server variables initialized successfully")
	})
	return initErr
}

// GetVariables returns the global variables instance
//func GetVariables() *Variables {
//	variablesMu.RLock()
//	defer variablesMu.RUnlock()
//	return globalVariables
//}

// GetSecretKey returns the current secret key
func GetSecretKey(store VariableStore) (string, error) {
	if globalConfig.Server.SecretKey != nil {
		return *globalConfig.Server.SecretKey, nil
	}

	generatedKey, err := utility.GenerateSecretKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate secret key: %w", err)
	}

	secretKey, err := GetOrCreateKey(store, SecretKeyRedisKey, generatedKey)
	if err != nil {
		return "", fmt.Errorf("failed to get secret key: %w", err)
	}
	return secretKey, nil
}

// SetSecretKey updates the secret key at runtime
//func SetSecretKey(key string) {
//	variablesMu.Lock()
//	defer variablesMu.Unlock()
//	if globalVariables != nil {
//		globalVariables.SecretKey = key
//		common.Info("Secret key updated at runtime")
//	}
//}

// GetOrCreateKey gets a key from store, or creates it if not exists
// - If key exists in store, returns the stored value
// - If key doesn't exist, calls createFn to generate value, stores it, and returns it
// - Uses SetNX to ensure atomic creation (only one caller succeeds when key doesn't exist)
func GetOrCreateKey(store VariableStore, key string, newValue string) (string, error) {
	if store == nil {
		err := fmt.Errorf("store is nil")
		common.Warn("VariableStore is nil, cannot get or create key", zap.String("key", key))
		return "store is nil", err
	}

	// Try to get existing value
	value, err := store.Get(key)
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
	if store.SetNX(key, newValue, SecretKeyTTL) {
		common.Info("New value stored successfully", zap.String("key", key))
		return newValue, nil
	}

	// Another process might have set it, try to get again
	value, err = store.Get(key)
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

// RefreshVariables refreshes all variables from storage
// Call this when you want to reload variables from persistent storage
func RefreshVariables(store VariableStore) error {
	if store == nil {
		return fmt.Errorf("store is nil")
	}

	variablesMu.Lock()
	defer variablesMu.Unlock()

	if globalVariables == nil {
		globalVariables = &Variables{}
	}

	// Refresh SecretKey
	secretKey, err := store.Get(SecretKeyRedisKey)
	if err != nil {
		common.Warn("Failed to refresh secret key from store", zap.Error(err))
		return err
	}
	if secretKey != "" {
		//globalVariables.SecretKey = secretKey
		common.Info("Secret key refreshed from store")
	}

	return nil
}

// VariableWatcher watches for variable changes in storage
// This can be used to detect changes made by other instances
type VariableWatcher struct {
	store    VariableStore
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewVariableWatcher creates a new variable watcher
func NewVariableWatcher(store VariableStore) *VariableWatcher {
	return &VariableWatcher{
		store:    store,
		stopChan: make(chan struct{}),
	}
}

// Start starts watching for variable changes
func (w *VariableWatcher) Start(interval time.Duration) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := RefreshVariables(w.store); err != nil {
					common.Debug("Failed to refresh variables", zap.Error(err))
				}
			case <-w.stopChan:
				return
			}
		}
	}()
	common.Info("Variable watcher started", zap.Duration("interval", interval))
}

// Stop stops the variable watcher
func (w *VariableWatcher) Stop() {
	close(w.stopChan)
	w.wg.Wait()
	common.Info("Variable watcher stopped")
}

// SaveToStorage saves current variables to persistent storage
func SaveToStorage(store VariableStore) error {
	if store == nil {
		return fmt.Errorf("store is nil")
	}

	variablesMu.RLock()
	defer variablesMu.RUnlock()

	if globalVariables == nil {
		return fmt.Errorf("variables not initialized")
	}

	// Save SecretKey
	//if !store.Set(SecretKeyRedisKey, globalVariables.SecretKey, SecretKeyTTL) {
	//	return fmt.Errorf("failed to save secret key to store")
	//}

	common.Info("Variables saved to storage")
	return nil
}

// WithTimeout creates a context with timeout for variable operations
func WithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
