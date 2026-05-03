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

package infinity

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	infinity "github.com/infiniflow/infinity-go-sdk"
	"ragflow/internal/server"
	"ragflow/internal/logger"
)

// infinityClient Infinity SDK client wrapper
type infinityClient struct {
	conn   *infinity.InfinityConnection
	dbName string
}

// NewInfinityClient creates a new Infinity client using the SDK
func NewInfinityClient(cfg *server.InfinityConfig) (*infinityClient, error) {
	// Parse URI like "localhost:23817" to get IP and port
	host := "127.0.0.1"
	port := 23817

	if cfg.URI != "" {
		parts := strings.Split(cfg.URI, ":")
		if len(parts) == 2 {
			host = parts[0]
			if p, err := strconv.Atoi(parts[1]); err == nil {
				port = p
			}
		}
	}

	// Retry connecting for up to 120 seconds (24 attempts * 5 seconds)
	logger.Info("Connecting to Infinity")
	var conn *infinity.InfinityConnection
	var err error
	for i := 0; i < 24; i++ {
		conn, err = infinity.Connect(infinity.NetworkAddress{IP: host, Port: port})
		if err == nil {
			break
		}
		if i < 23 {
			time.Sleep(5 * time.Second)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to Infinity after 120s: %w", err)
	}

	client := &infinityClient{
		conn:   conn,
		dbName: cfg.DBName,
	}

	return client, nil
}

// WaitForHealthy blocks until Infinity is healthy or timeout
func (c *infinityClient) WaitForHealthy(ctx context.Context, timeout time.Duration) error {
	logger.Info("Waiting for Infinity to be healthy")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		res, err := c.conn.ShowCurrentNode()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		// Use reflection to access ErrorCode and ServerStatus fields
		// since ShowCurrentNodeResponse is in an internal package
		v := reflect.ValueOf(res)
		if v.Kind() != reflect.Ptr {
			time.Sleep(5 * time.Second)
			continue
		}
		v = v.Elem()
		errorCode := v.FieldByName("ErrorCode")
		serverStatus := v.FieldByName("ServerStatus")
		if !errorCode.IsValid() || !serverStatus.IsValid() {
			time.Sleep(5 * time.Second)
			continue
		}
		// ErrorCode 0 means OK, ServerStatus "started" or "alive" means healthy
		if errorCode.Int() == 0 {
			status := serverStatus.String()
			if status == "started" || status == "alive" {
				logger.Info("Infinity is healthy")
				return nil
			}
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("Infinity not healthy after %v", timeout)
}

// Engine Infinity engine implementation using Go SDK
type infinityEngine struct {
	config                  *server.InfinityConfig
	client                 *infinityClient
	mappingFileName        string
	docMetaMappingFileName string
}

// NewEngine creates an Infinity engine
func NewEngine(cfg interface{}) (*infinityEngine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("infinity config is nil, please check your configuration file for 'doc_engine.infinity' settings")
	}
	infConfig, ok := cfg.(*server.InfinityConfig)
	if !ok {
		return nil, fmt.Errorf("invalid infinity config type, expected *config.InfinityConfig")
	}
	if infConfig == nil {
		return nil, fmt.Errorf("infinity config is nil, please check your configuration file for 'doc_engine.infinity' settings")
	}

	client, err := NewInfinityClient(infConfig)
	if err != nil {
		return nil, err
	}

	mappingFileName := infConfig.MappingFileName
	if mappingFileName == "" {
		mappingFileName = "infinity_mapping.json"
	}
	docMetaMappingFileName := infConfig.DocMetaMappingFileName
	if docMetaMappingFileName == "" {
		docMetaMappingFileName = "doc_meta_infinity_mapping.json"
	}

	engine := &infinityEngine{
		config:              infConfig,
		client:              client,
		mappingFileName:     mappingFileName,
		docMetaMappingFileName: docMetaMappingFileName,
	}

	// Wait for Infinity to be healthy
	if err := client.WaitForHealthy(context.Background(), 120*time.Second); err != nil {
		return nil, fmt.Errorf("Infinity not healthy: %w", err)
	}

	// MigrateDB creates the database if it doesn't exist
	if err := engine.MigrateDB(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return engine, nil
}

// GetType returns the engine type
func (e *infinityEngine) GetType() string {
	return "infinity"
}

// Ping checks if Infinity is accessible
func (e *infinityEngine) Ping(ctx context.Context) error {
	if e.client == nil || e.client.conn == nil {
		return fmt.Errorf("Infinity client not initialized")
	}
	if !e.client.conn.IsConnected() {
		return fmt.Errorf("Infinity not connected")
	}
	return nil
}

// Close closes the Infinity connection
func (e *infinityEngine) Close() error {
	if e.client != nil && e.client.conn != nil {
		_, err := e.client.conn.Disconnect()
		return err
	}
	return nil
}

// MigrateDB creates the database if it doesn't exist
func (e *infinityEngine) MigrateDB(ctx context.Context) error {
	_, err := e.client.conn.CreateDatabase(e.client.dbName, infinity.ConflictTypeIgnore, "")
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	return nil
}
