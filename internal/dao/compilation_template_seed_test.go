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

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestBuiltinCompilationTemplateKinds_NoDatasetnav locks the removal of the
// datasetnav template from the built-in catalogue: dataset navigation is not an
// independent compile kind (see component_test.TestKnowledgeCompiler_Datasetnav_NoVariant),
// so it must not appear in the seeded template kinds.
func TestBuiltinCompilationTemplateKinds_NoDatasetnav(t *testing.T) {
	for _, k := range builtinCompilationTemplateKinds {
		if k.Kind == "datasetnav" || k.Kind == "dataset_nav" {
			t.Fatalf("builtin compilation template kind %q must not exist (datasetnav is a by-product, not a compile kind)", k.Kind)
		}
	}
}

// TestBuiltinTemplateIDWithinColumnWidth guards against a regression where the
// built-in template id exceeds the varchar(32) compilation_template.id column,
// which would make the MySQL seed fail with Error 1406 (data too long).
func TestBuiltinTemplateIDWithinColumnWidth(t *testing.T) {
	const idColumnSize = 32
	seen := make(map[string]string, len(builtinCompilationTemplateKinds))
	for _, k := range builtinCompilationTemplateKinds {
		id := builtinTemplateID(k.Kind)
		if len(id) > idColumnSize {
			t.Fatalf("built-in template id %q is %d bytes > id column size %d", id, len(id), idColumnSize)
		}
		// Truncated ids must stay unique so OnConflict(id) does not clobber two
		// kinds into one row.
		if prev, dup := seen[id]; dup {
			t.Fatalf("built-in template id %q collides for kinds %q and %q", id, prev, k.Kind)
		}
		seen[id] = k.Kind
	}
}

func TestSeedBuiltinCompilationTemplatesForTenant(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.CompilationTemplateGroup{}, &entity.CompilationTemplate{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// ResolveGroupTemplateIDs reads the package-global DB, so point it at our
	// sqlite instance for the duration of the test.
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	ctx := t.Context()
	const tenantID = "tenant-1"

	// Idempotent across repeated calls, and the built-in group is always a
	// global (tenant_id = "") catalogue regardless of the tenantID argument.
	for i := 0; i < 2; i++ {
		if err = SeedBuiltinCompilationTemplatesForTenant(ctx, db, tenantID); err != nil {
			t.Fatalf("seed (iter %d): %v", i, err)
		}
	}

	var groups []entity.CompilationTemplateGroup
	if err = db.Find(&groups).Error; err != nil {
		t.Fatalf("read groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	if groups[0].ID != BuiltinCompilationTemplateGroupID || groups[0].TenantID != "" {
		t.Fatalf("unexpected group: %#v", groups[0])
	}

	var templates []entity.CompilationTemplate
	if err = db.Find(&templates).Error; err != nil {
		t.Fatalf("read templates: %v", err)
	}
	if len(templates) != len(builtinCompilationTemplateKinds) {
		t.Fatalf("got %d templates, want %d", len(templates), len(builtinCompilationTemplateKinds))
	}
	for _, tmpl := range templates {
		// Built-in templates are a global catalogue (tenant_id nil).
		if tmpl.TenantID != nil {
			t.Errorf("template %s: tenant_id=%v, want nil (global catalogue)", tmpl.ID, tmpl.TenantID)
		}
		if tmpl.GroupID == nil || *tmpl.GroupID != BuiltinCompilationTemplateGroupID {
			t.Errorf("template %s: group_id=%v, want %q", tmpl.ID, tmpl.GroupID, BuiltinCompilationTemplateGroupID)
		}
		if tmpl.Kind == "" {
			t.Errorf("template %s: empty kind", tmpl.ID)
		}
		if tmpl.Config["kind"] != tmpl.Kind {
			t.Errorf("template %s: config.kind=%v, want %q", tmpl.ID, tmpl.Config["kind"], tmpl.Kind)
		}
	}

	// Re-seeding with a different tenant is idempotent for the global group:
	// the single built-in group row stays (id is the PK), and templates are
	// not duplicated.
	if err = SeedBuiltinCompilationTemplatesForTenant(ctx, db, "tenant-2"); err != nil {
		t.Fatalf("seed tenant-2: %v", err)
	}
	var allGroups []entity.CompilationTemplateGroup
	if err = db.Find(&allGroups).Error; err != nil {
		t.Fatalf("read all groups: %v", err)
	}
	if len(allGroups) != 1 {
		t.Fatalf("got %d groups, want 1 (built-in group is global, single row)", len(allGroups))
	}
	if allGroups[0].TenantID != "" {
		t.Errorf("built-in group tenant_id = %q, want empty (global catalogue)", allGroups[0].TenantID)
	}

	// ResolveGroupTemplateIDs resolves the built-in group for any tenant.
	resolved, err := NewCompilationTemplateDAO().ResolveGroupTemplateIDs(ctx, db, "some-other-tenant", []string{BuiltinCompilationTemplateGroupID})
	if err != nil {
		t.Fatalf("resolve built-in group: %v", err)
	}
	if len(resolved) != len(builtinCompilationTemplateKinds) {
		t.Fatalf("resolved %d template ids, want %d", len(resolved), len(builtinCompilationTemplateKinds))
	}
}
