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

package oceanbase

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestInitializeConfiguresIndexRefresh(t *testing.T) {
	tests := []struct {
		name          string
		engineType    string
		serverVersion string
		wantEnabled   bool
	}{
		{
			name:          "SeekDB 1.3 enables refresh",
			engineType:    "seekdb",
			serverVersion: "5.7.25-OceanBase seekdb-v1.3.0.0",
			wantEnabled:   true,
		},
		{
			name:          "SeekDB 1.2 keeps refresh disabled",
			engineType:    "seekdb",
			serverVersion: "5.7.25-OceanBase seekdb-v1.2.0.0",
			wantEnabled:   false,
		},
		{
			name:       "OceanBase keeps refresh disabled",
			engineType: "oceanbase",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ENABLE_HYBRID_SEARCH", "false")
			t.Setenv("OB_QUERY_TIMEOUT", "100000000")
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			engine := newEngineWithDB(test.engineType, "legacy_doc", db)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT OB_VERSION()")).
				WillReturnRows(sqlmock.NewRows([]string{"OB_VERSION()"}).AddRow("4.3.5.3"))
			if test.engineType == "seekdb" {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
					WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow(test.serverVersion))
			}
			mock.ExpectQuery(regexp.QuoteMeta("SHOW VARIABLES LIKE 'ob_query_timeout'")).
				WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).AddRow("ob_query_timeout", 100000000))

			if err := engine.initialize(context.Background()); err != nil {
				t.Fatal(err)
			}
			if engine.indexRefreshEnabled != test.wantEnabled {
				t.Fatalf("indexRefreshEnabled = %t, want %t", engine.indexRefreshEnabled, test.wantEnabled)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInitializeDisablesRefreshForUnrecognizedSeekDBVersion(t *testing.T) {
	t.Setenv("ENABLE_HYBRID_SEARCH", "false")
	t.Setenv("OB_QUERY_TIMEOUT", "100000000")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := newEngineWithDB("seekdb", "legacy_doc", db)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT OB_VERSION()")).
		WillReturnRows(sqlmock.NewRows([]string{"OB_VERSION()"}).AddRow("4.3.5.3"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
		WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("5.7.25-OceanBase"))
	mock.ExpectQuery(regexp.QuoteMeta("SHOW VARIABLES LIKE 'ob_query_timeout'")).
		WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).AddRow("ob_query_timeout", 100000000))

	if err := engine.initialize(context.Background()); err != nil {
		t.Fatalf("initialize() error = %v", err)
	}
	if engine.indexRefreshEnabled {
		t.Fatal("indexRefreshEnabled = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExtractSeekDBVersion(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "5.7.25-OceanBase seekdb-v1.3.0.0", want: "1.3.0.0", ok: true},
		{raw: "OceanBase SEEKDB 1.3.0", want: "1.3.0", ok: true},
		{raw: "OceanBase_CE 4.3.5.3", ok: false},
	} {
		got, ok := extractSeekDBVersion(test.raw)
		if got != test.want || ok != test.ok {
			t.Errorf("extractSeekDBVersion(%q) = (%q, %t), want (%q, %t)", test.raw, got, ok, test.want, test.ok)
		}
	}
}
