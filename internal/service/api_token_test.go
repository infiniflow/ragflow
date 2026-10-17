package service

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// TestMaskAPIToken pins the masking contract for the api_keys list leak fix
// (#20555): the masked form must never echo the body of the token, must keep
// a recognizable prefix/suffix pair for realistic ragflow tokens, and must
// collapse empty / too-short secrets to constants so a no-record field stays
// distinguishable from a masked one. Same shape as maskAPIKey (#20549).
func TestMaskAPIToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty stays empty", in: "", want: ""},
		{name: "one char is too short, never echoed", in: "x", want: "***"},
		{name: "seven chars is too short, never echoed", in: "abcdefg", want: "***"},
		{name: "eight chars exposes only prefix+suffix", in: "abcdefgh", want: "abc***efgh"},
		{name: "ragflow token keeps the rag prefix and tail", in: "ragflow-AbCdEfGhIjKlMnOpQrStUvWxYz012345", want: "rag***2345"},
		{name: "raw 32-char secret exposes only ends", in: "0123456789abcdef0123456789abcdef", want: "012***cdef"},
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
	if want := "rag***3210"; keys[0].Token != want {
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
