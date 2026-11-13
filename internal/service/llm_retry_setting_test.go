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

package service

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

func TestApplyLLMMaxRetriesSetting(t *testing.T) {
	const configured = 4
	tests := []struct {
		name    string
		rows    []entity.SystemSettings
		want    int
		wantErr bool
	}{
		{name: "no row uses the configured count", want: configured},
		{name: "empty value uses the configured count", rows: []entity.SystemSettings{{Name: common.SettingLLMMaxRetries, Value: ""}}, want: configured},
		{name: "override replaces the configured count", rows: []entity.SystemSettings{{Name: common.SettingLLMMaxRetries, Value: "7"}}, want: 7},
		{name: "zero disables retries", rows: []entity.SystemSettings{{Name: common.SettingLLMMaxRetries, Value: "0"}}, want: 0},
		{name: "out of range keeps the count in effect", rows: []entity.SystemSettings{{Name: common.SettingLLMMaxRetries, Value: "99"}}, want: 2, wantErr: true},
		{name: "non-numeric keeps the count in effect", rows: []entity.SystemSettings{{Name: common.SettingLLMMaxRetries, Value: "many"}}, want: 2, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { _ = common.SetLLMMaxRetries(common.DefaultLLMMaxRetries) })
			if err := common.SetLLMMaxRetries(2); err != nil {
				t.Fatal(err)
			}
			db := testutil.SetupTestDB(t, &entity.SystemSettings{})
			for _, row := range tt.rows {
				row.DataType = "integer"
				if err := db.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
			}

			err := ApplyLLMMaxRetriesSetting(context.Background(), db, configured)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got := common.LLMMaxRetries(); got != tt.want {
				t.Errorf("LLMMaxRetries() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestApplyLLMMaxRetriesSettingFollowsEdits pins the runtime behavior: a saved
// override takes effect on the next sync, and clearing it returns to the
// configured count.
func TestApplyLLMMaxRetriesSettingFollowsEdits(t *testing.T) {
	t.Cleanup(func() { _ = common.SetLLMMaxRetries(common.DefaultLLMMaxRetries) })
	db := testutil.SetupTestDB(t, &entity.SystemSettings{})
	row := entity.SystemSettings{Name: common.SettingLLMMaxRetries, Value: "", DataType: "integer"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		value string
		want  int
	}{{"", 3}, {"8", 8}, {"0", 0}, {"", 3}}
	for _, step := range steps {
		if err := db.Model(&entity.SystemSettings{}).Where("name = ?", common.SettingLLMMaxRetries).Update("value", step.value).Error; err != nil {
			t.Fatal(err)
		}
		if err := ApplyLLMMaxRetriesSetting(context.Background(), db, 3); err != nil {
			t.Fatal(err)
		}
		if got := common.LLMMaxRetries(); got != step.want {
			t.Fatalf("after saving %q: LLMMaxRetries() = %d, want %d", step.value, got, step.want)
		}
	}
}

// TestSyncLLMMaxRetriesAppliesImmediatelyAndStopsWithItsContext covers the
// start-up path: the stored override is in effect before the first tick, and
// the loop ends when the service shuts down.
func TestSyncLLMMaxRetriesAppliesImmediatelyAndStopsWithItsContext(t *testing.T) {
	t.Cleanup(func() { _ = common.SetLLMMaxRetries(common.DefaultLLMMaxRetries) })
	db := testutil.SetupTestDB(t, &entity.SystemSettings{})
	row := entity.SystemSettings{Name: common.SettingLLMMaxRetries, Value: "6", DataType: "integer"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		SyncLLMMaxRetries(ctx, db, 3)
		close(done)
	}()

	deadline := time.After(5 * time.Second)
	for common.LLMMaxRetries() != 6 {
		select {
		case <-deadline:
			t.Fatalf("LLMMaxRetries() = %d, want the stored override 6", common.LLMMaxRetries())
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SyncLLMMaxRetries did not stop after its context ended")
	}
}
