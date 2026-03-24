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
	"ragflow/internal/server"
	"strconv"
	"strings"

	infinity "github.com/infiniflow/infinity-go-sdk"
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

	conn, err := infinity.Connect(infinity.NetworkAddress{IP: host, Port: port})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Infinity: %w", err)
	}

	return &infinityClient{
		conn:   conn,
		dbName: cfg.DBName,
	}, nil
}

// Engine Infinity engine implementation using Go SDK
type infinityEngine struct {
	config *server.InfinityConfig
	client *infinityClient
}

// NewEngine creates an Infinity engine
func NewEngine(cfg interface{}) (*infinityEngine, error) {
	infConfig, ok := cfg.(*server.InfinityConfig)
	if !ok {
		return nil, fmt.Errorf("invalid infinity config type, expected *config.InfinityConfig")
	}

	client, err := NewInfinityClient(infConfig)
	if err != nil {
		return nil, err
	}

	engine := &infinityEngine{
		config: infConfig,
		client: client,
	}

	return engine, nil
}

// Type returns the engine type
func (e *infinityEngine) Type() string {
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
