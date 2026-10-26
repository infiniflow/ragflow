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
	"errors"
	"reflect"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestApplyVersionDraftSave(t *testing.T) {
	for _, scenario := range []string{"repeat", "version_save_failure", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			setupAgentSessionServiceTest(t)
			current := entity.UserCanvas{
				ID: "canvas-1", UserID: "user-1", Release: true,
				DSL: entity.JSONMap{"components": map[string]interface{}{}, "revision": "current"},
			}
			if err := dao.DB.Create(&current).Error; err != nil {
				t.Fatal(err)
			}
			createdAt := int64(1)
			version := entity.UserCanvasVersion{
				ID: "version-1", UserCanvasID: current.ID, Release: true,
				DSL:       entity.JSONMap{"components": map[string]interface{}{}, "revision": "old"},
				BaseModel: entity.BaseModel{CreateTime: &createdAt},
			}
			if err := dao.DB.Create(&version).Error; err != nil {
				t.Fatal(err)
			}
			latestCreatedAt := int64(2)
			if err := dao.DB.Create(&entity.UserCanvasVersion{
				ID: "version-2", UserCanvasID: current.ID, Release: true, DSL: current.DSL,
				BaseModel: entity.BaseModel{CreateTime: &latestCreatedAt},
			}).Error; err != nil {
				t.Fatal(err)
			}

			ctx := serviceTestContext(t, "user-1", "user-1")
			var wantErr error
			if scenario == "version_save_failure" {
				wantErr = errors.New("version write failed")
				if err := dao.DB.Callback().Create().Before("gorm:create").Register("fail_version_save", func(tx *gorm.DB) {
					if tx.Statement.Table == "user_canvas_version" {
						tx.AddError(wantErr)
					}
				}); err != nil {
					t.Fatal(err)
				}
			} else if scenario == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				wantErr = context.Canceled
			}
			svc := NewAgentService()
			err := svc.ApplyVersion(ctx, "user-1", current.ID, version.ID)
			if !errors.Is(err, wantErr) {
				t.Fatalf("ApplyVersion error = %v, want %v", err, wantErr)
			}
			wantDSL, wantRelease, wantCount := current.DSL, true, 2
			if wantErr == nil {
				latest, err := dao.NewUserCanvasVersionDAO().GetLatest(ctx, dao.DB, current.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := svc.ApplyVersion(ctx, "user-1", current.ID, version.ID); err != nil {
					t.Fatalf("repeated ApplyVersion failed: %v", err)
				}
				after, err := dao.NewUserCanvasVersionDAO().GetLatest(ctx, dao.DB, current.ID)
				if err != nil {
					t.Fatal(err)
				}
				if after.ID != latest.ID || after.Release {
					t.Fatal("repeated apply did not reuse the latest draft")
				}
				wantDSL, wantRelease, wantCount = version.DSL, false, 3
			}
			persisted, err := dao.NewUserCanvasDAO().GetByID(t.Context(), dao.DB, current.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(persisted.DSL, wantDSL) || persisted.Release != wantRelease {
				t.Fatalf("unexpected agent after apply: dsl=%v release=%v", persisted.DSL, persisted.Release)
			}
			versions, err := dao.NewUserCanvasVersionDAO().ListByCanvasID(t.Context(), dao.DB, current.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(versions) != wantCount {
				t.Fatalf("version count = %d, want %d", len(versions), wantCount)
			}
		})
	}
}
