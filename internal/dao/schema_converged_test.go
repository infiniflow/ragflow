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

	"ragflow/internal/common"
)

func TestSchemaConvergedForBuild(t *testing.T) {
	cases := []struct {
		name        string
		migrate     bool   // create the system_settings table
		marker      string // pre-seeded marker value; "" seeds nothing
		codeVersion string
		want        bool
	}{
		{name: "no system_settings table", migrate: false, codeVersion: "v1.0.0-rc1", want: false},
		{name: "no marker row", migrate: true, codeVersion: "v1.0.0-rc1", want: false},
		{name: "marker matches build", migrate: true, marker: "v1.0.0-rc1", codeVersion: "v1.0.0-rc1", want: true},
		{name: "marker from older build", migrate: true, marker: "v1.0.0-rc1", codeVersion: "v1.0.0-rc2", want: false},
		{name: "unrecordable build never skips", migrate: true, marker: "unknown", codeVersion: "unknown", want: false},
		{name: "empty build never skips", migrate: true, marker: "", codeVersion: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupMigrationVersionTestDB(t, tc.migrate)
			ctx := t.Context()
			if tc.marker != "" {
				if err := markSchemaConverged(ctx, db, tc.marker); err != nil {
					t.Fatalf("markSchemaConverged() error = %v", err)
				}
			}
			if got := schemaConvergedForBuild(ctx, db, tc.codeVersion); got != tc.want {
				t.Fatalf("schemaConvergedForBuild() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSchemaConvergedForBuildDevModeNeverSkips(t *testing.T) {
	t.Setenv(common.EnvRAGFlowDevMode, "true")
	db := setupMigrationVersionTestDB(t, true)
	ctx := t.Context()
	if err := markSchemaConverged(ctx, db, "v1.0.0-rc1"); err != nil {
		t.Fatalf("markSchemaConverged() error = %v", err)
	}
	if schemaConvergedForBuild(ctx, db, "v1.0.0-rc1") {
		t.Fatal("schemaConvergedForBuild() = true in dev mode, want false")
	}
}

func TestMarkSchemaConverged(t *testing.T) {
	db := setupMigrationVersionTestDB(t, true)
	ctx := t.Context()

	// Unrecordable build strings write nothing.
	for _, v := range []string{"", "unknown"} {
		if err := markSchemaConverged(ctx, db, v); err != nil {
			t.Fatalf("markSchemaConverged(%q) error = %v", v, err)
		}
	}
	if got, err := readSchemaConvergedVersion(ctx, db); err != nil || got != "" {
		t.Fatalf("readSchemaConvergedVersion() = %q, %v; want empty", got, err)
	}

	// A real build writes the marker, and a later build overwrites it.
	if err := markSchemaConverged(ctx, db, "v1.0.0-rc1"); err != nil {
		t.Fatalf("markSchemaConverged() error = %v", err)
	}
	if err := markSchemaConverged(ctx, db, "v1.0.0-rc2"); err != nil {
		t.Fatalf("markSchemaConverged() second error = %v", err)
	}
	if got, err := readSchemaConvergedVersion(ctx, db); err != nil || got != "v1.0.0-rc2" {
		t.Fatalf("readSchemaConvergedVersion() = %q, %v; want v1.0.0-rc2", got, err)
	}
}

func TestMarkSchemaConvergedWithoutTable(t *testing.T) {
	db := setupMigrationVersionTestDB(t, false)
	if err := markSchemaConverged(t.Context(), db, "v1.0.0-rc1"); err != nil {
		t.Fatalf("markSchemaConverged() without table error = %v", err)
	}
}

func TestMarkSchemaConvergedDevModeWritesNothing(t *testing.T) {
	t.Setenv(common.EnvRAGFlowDevMode, "true")
	db := setupMigrationVersionTestDB(t, true)
	ctx := t.Context()
	if err := markSchemaConverged(ctx, db, "v1.0.0-rc1"); err != nil {
		t.Fatalf("markSchemaConverged() error = %v", err)
	}
	if got, err := readSchemaConvergedVersion(ctx, db); err != nil || got != "" {
		t.Fatalf("readSchemaConvergedVersion() = %q, %v; want empty in dev mode", got, err)
	}
}
