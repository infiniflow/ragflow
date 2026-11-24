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

package dao

import (
	"database/sql"
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// OpenIndependentMySQLDB opens a separate pool with the active connection
// settings. Session locks must not reserve connections from the application
// pool while their owners run database operations through that same pool.
func OpenIndependentMySQLDB(db *gorm.DB) (*sql.DB, error) {
	var config *mysql.Config
	switch dialector := db.Dialector.(type) {
	case migrationAwareDialector:
		config = dialector.Config
	case *mysql.Dialector:
		config = dialector.Config
	default:
		return nil, fmt.Errorf("independent session requires a MySQL dialector")
	}
	if config == nil || config.DSN == "" {
		return nil, fmt.Errorf("independent session requires a MySQL DSN")
	}
	driverName := config.DriverName
	if driverName == "" {
		driverName = "mysql"
	}
	pool, err := sql.Open(driverName, config.DSN)
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(0)
	return pool, nil
}
