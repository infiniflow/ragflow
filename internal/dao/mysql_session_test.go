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
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestIndependentMySQLDBSupportsMigrationDialector(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		name := "mysql"
		if wrapped {
			name = "migration wrapper"
		}
		t.Run(name, func(t *testing.T) {
			shared, _, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { shared.Close() })
			base := mysql.New(mysql.Config{DSN: "test:test@tcp(localhost:3306)/test?parseTime=True", Conn: shared, SkipInitializeWithVersion: true}).(*mysql.Dialector)
			var dialect gorm.Dialector = base
			if wrapped {
				dialect = migrationAwareDialector{Dialector: base}
			}
			db, err := gorm.Open(dialect, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			independent, err := OpenIndependentMySQLDB(db)
			if err != nil {
				t.Fatal(err)
			}
			defer independent.Close()
			if independent == shared || independent.Stats().MaxOpenConnections != 1 {
				t.Fatal("session lock must have its own bounded connection pool")
			}
		})
	}
}
