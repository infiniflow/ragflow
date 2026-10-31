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
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestCreateAPIKeyStoresUserAndTenantContext(t *testing.T) {
	ctx := t.Context()
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.APIToken{}); err != nil {
		t.Fatalf("failed to migrate api_token: %v", err)
	}
	pushServiceDB(t, db)

	key, err := (&SystemService{}).CreateAPIKey(ctx, "user-1", "tenant-1", &CreateAPIKeyRequest{})
	if err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}
	if key.UserID != "user-1" || key.TenantID != "tenant-1" {
		t.Fatalf("CreateAPIKey() context = (%q, %q), want (user-1, tenant-1)", key.UserID, key.TenantID)
	}

	stored, err := dao.NewAPITokenDAO().GetByAPIToken(ctx, dao.DB, key.Token)
	if err != nil {
		t.Fatalf("GetByAPIToken() error = %v", err)
	}
	if stored == nil || stored.UserID == nil || *stored.UserID != "user-1" || stored.TenantID != "tenant-1" {
		t.Fatalf("stored API token context = %+v, want user-1 in tenant-1", stored)
	}
}

// TestMaskAPIToken pins the masking contract for the api_keys list leak fix
// (#20555): the masked form must never echo the body of the token, must keep
// the four-character prefix/suffix pair for realistic ragflow tokens, and
// must collapse empty secrets and anything a 4+4 split would fully reveal
// (eight characters or fewer) to constants, so a no-record field stays
// distinguishable from a masked one.
func TestMaskAPIToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty stays empty", in: "", want: ""},
		{name: "one char is too short, never echoed", in: "x", want: "***"},
		{name: "seven chars is too short, never echoed", in: "abcdefg", want: "***"},
		{name: "eight chars would be fully revealed by a 4+4 split, collapse", in: "abcdefgh", want: "***"},
		{name: "nine chars exposes exactly 4+4", in: "abcdefghi", want: "abcd***fghi"},
		{name: "ragflow token keeps the ragf prefix and tail", in: "ragflow-AbCdEfGhIjKlMnOpQrStUvWxYz012345", want: "ragf***2345"},
		{name: "raw 32-char secret exposes only ends", in: "0123456789abcdef0123456789abcdef", want: "0123***cdef"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := maskAPIToken(c.in); got != c.want {
				t.Errorf("maskAPIToken(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func setupAPITokenTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.APIToken{}); err != nil {
		t.Fatalf("failed to migrate test schema: %v", err)
	}

	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() {
		dao.DB = origDB
	})

	return db
}

func createAPITokenRow(t *testing.T, db *gorm.DB, tenantID, token string, beta *string) {
	t.Helper()

	row := &entity.APIToken{
		TenantID: tenantID,
		Token:    token,
		Beta:     beta,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("failed to create api token: %v", err)
	}
}

// TestListAPIKeysMasksToken is the regression test for #20555: the list
// endpoint must never echo the full credential, while CreateAPIKey's
// one-time full-token response remains the copy-to-clipboard source. The
// stored row is untouched — masking happens only at response build time —
// and a beta-less legacy row still gets its lazily backfilled beta in full,
// because this list response is the only place that backfilled value is
// ever revealed.
func TestListAPIKeysMasksToken(t *testing.T) {
	db := setupAPITokenTestDB(t)

	fullToken := "ragflow-ZxYwVuTsRqPoNmLkJiHgFeDcBa9876543210"
	existingBeta := "beta0123456789abcdef0123456789abcdef"
	createAPITokenRow(t, db, "tenant-1", fullToken, &existingBeta)
	// A legacy row with no beta exercises the lazy-backfill path under masking.
	createAPITokenRow(t, db, "tenant-1", "ragflow-legacy0123456789abcdefghij", nil)

	svc := NewSystemService()
	keys, err := svc.ListAPIKeys(t.Context(), "tenant-1")
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(keys))
	}

	for _, key := range keys {
		if key.Token == fullToken || key.Token == "ragflow-legacy0123456789abcdefghij" {
			t.Errorf("list response echoes the full token %q", key.Token)
		}
		if !strings.Contains(key.Token, "***") {
			t.Errorf("token %q is not masked", key.Token)
		}
	}
	if want := "ragf***3210"; keys[0].Token != want {
		t.Errorf("masked token = %q, want %q", keys[0].Token, want)
	}

	if keys[0].Beta == nil || *keys[0].Beta != existingBeta {
		t.Errorf("existing beta = %v, want the stored value untouched", keys[0].Beta)
	}
	if keys[1].Beta == nil || *keys[1].Beta == "" {
		t.Error("legacy row did not get a backfilled beta in the response")
	}

	var stored entity.APIToken
	if err := db.Where("token = ?", fullToken).First(&stored).Error; err != nil {
		t.Fatalf("read back stored row: %v", err)
	}
	if stored.Token != fullToken {
		t.Errorf("stored token = %q, want the original %q (masking must not write back)", stored.Token, fullToken)
	}
}
