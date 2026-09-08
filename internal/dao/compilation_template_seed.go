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
	"context"
	"ragflow/internal/common"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BuiltinCompilationTemplateGroupID is the id of the system-provided
// compilation template group that compiler.json references as its default
// "create from template" group. It is a global (tenant-agnostic) catalogue so
// any tenant can resolve it without depending on Python-side initialization.
const BuiltinCompilationTemplateGroupID = "c3aa748c8b2111f191f3047c16ec874f"

// builtinCompilationTemplateKinds is the set of variants shipped in the
// built-in group. The Go KnowledgeCompiler derives the variant from each
// template's kind via common.KindToVariant.
var builtinCompilationTemplateKinds = []struct {
	Kind string
	Name string
}{
	{Kind: "tree", Name: "Tree — RAPTOR-based document tree"},
	{Kind: "mind_map", Name: "Mind map - Radial concept hierarchy"},
	{Kind: "wiki", Name: "Wiki — Graph-based wiki"},
	{Kind: "knowledge_graph", Name: "Knowledge graph"},
	{Kind: "page_index", Name: "Page index"},
	{Kind: "session_essence", Name: "Session essence"},
	{Kind: "session_graph", Name: "Session graph"},
	{Kind: "timeline", Name: "Timeline"},
}

func strPTR(s string) *string { return &s }

// builtinTemplateID derives a deterministic, <=32-byte id for a built-in
// compilation template from "<builtin-group-id>-<kind>". A plain concatenation
// overflows the varchar(32) id column for long kinds (e.g. knowledge_graph),
// so we take the last 32 bytes. Each kind ends up inside those last bytes (the
// "-<kind>" suffix is preserved and kinds are distinct), so the truncated ids
// stay unique and readable, and the seed stays idempotent (OnConflict on id).
func builtinTemplateID(kind string) string {
	full := BuiltinCompilationTemplateGroupID + "-" + kind
	if len(full) <= 32 {
		return full
	}
	return full[len(full)-32:]
}

// SeedBuiltinCompilationTemplates provisions the built-in compilation template
// group (and its child templates) as a global, tenant-agnostic catalogue, so
// the Go compiler (and the frontend "create from template" catalogue) can
// resolve compiler.json's default group id out of the box. The group row uses
// an empty tenant_id and ResolveGroupTemplateIDs treats this built-in id as
// resolvable for every tenant.
func SeedBuiltinCompilationTemplates(ctx context.Context, db *gorm.DB) error {
	if err := SeedBuiltinCompilationTemplatesForTenant(ctx, db, ""); err != nil {
		common.Warn("Failed to seed built-in compilation templates", zap.Error(err))
		return err
	}
	return nil
}

// SeedBuiltinCompilationTemplatesForTenant upserts the built-in compilation
// template group and its child templates as a global, tenant-agnostic
// catalogue. The tenantID argument is accepted for API symmetry but the
// built-in entries always use an empty tenant_id, because ResolveGroupTemplateIDs
// resolves the built-in group id for every tenant. It is idempotent
// (OnConflict DO NOTHING on id) so it is safe to call repeatedly.
func SeedBuiltinCompilationTemplatesForTenant(ctx context.Context, db *gorm.DB, tenantID string) error {
	valid := string(entity.StatusValid)
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group := &entity.CompilationTemplateGroup{
			ID:       BuiltinCompilationTemplateGroupID,
			TenantID: "", // global built-in catalogue
			Name:     "Built-in templates",
			Status:   &valid,
		}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoNothing: true,
		}).Create(group).Error; err != nil {
			return err
		}

		for _, t := range builtinCompilationTemplateKinds {
			tmpl := &entity.CompilationTemplate{
				ID:       builtinTemplateID(t.Kind),
				TenantID: nil, // global built-in catalogue
				GroupID:  strPTR(BuiltinCompilationTemplateGroupID),
				Name:     t.Name,
				Kind:     t.Kind,
				Config:   entity.JSONMap{"kind": t.Kind},
				Status:   &valid,
			}
			if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				DoNothing: true,
			}).Create(tmpl).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
