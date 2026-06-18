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

package tool

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// sqlmockDialer creates a sqlmock-backed dialer for ExeSQL tests.
// It returns:
//   - dialer: an exesqlDialer that returns the sqlmock DB.
//   - mock:   sqlmock.Sqlmock for staging expectations.
//   - cleanup: a func() that closes the DB.
func sqlmockDialer(t *testing.T) (exesqlDialer, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	dialer := func(driver, dsn string) (*sql.DB, error) {
		return db, nil
	}
	return dialer, mock, func() { db.Close() }
}
