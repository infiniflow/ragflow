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

package clickhouse

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

var (
	globalDriver *Driver
	once         sync.Once
)

// GetDriver gets global ClickHouse client instance
func GetDriver() *Driver {
	return globalDriver
}

type Driver struct {
	conn   driver.Conn
	config *clickhouse.Options
}

func Init(ctx context.Context, host string, port int, user, password, database string) error {
	var err error
	once.Do(func() {
		address := fmt.Sprintf("%s:%d", host, port)

		globalDriver = &Driver{}
		globalDriver.config = &clickhouse.Options{
			Addr: []string{address},
			Auth: clickhouse.Auth{
				Database: database,
				Username: user,
				Password: password,
			},
			DialTimeout:     5 * time.Second,
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: time.Hour,
		}

		globalDriver.conn, err = clickhouse.Open(globalDriver.config)
		if err != nil {
			return
		}

		if err = globalDriver.conn.Ping(ctx); err != nil {
			return
		}
	})
	return err
}

func (d *Driver) Ping(ctx context.Context) error {
	return d.conn.Ping(ctx)
}

func (d *Driver) Status() (map[string]interface{}, error) {
	stats := d.conn.Stats()
	version, err := d.conn.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get server version: %w", err)
	}
	return map[string]interface{}{
		"max_open_conns": stats.MaxOpenConns,
		"max_idle_conns": stats.MaxIdleConns,
		"open":           stats.Open,
		"idle":           stats.Idle,
		"server_version": version.String(),
	}, nil
}

func Close() error {
	return globalDriver.conn.Close()
}
