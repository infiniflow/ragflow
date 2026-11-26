package dao

import (
	"context"
	"regexp"
	"testing"
	"time"

	"ragflow/internal/entity"
)

func sqliteTestPtr[T any](v T) *T { return &v }

// The team page's member list is the query that first hit SQLite's missing
// DATE_FORMAT; the desktop build has to render the same timestamp shape as the
// MySQL path.
func TestGetMembersByTenantIDOnSQLite(t *testing.T) {
	db := newSQLiteTestDB(t)
	ctx := context.Background()
	for _, model := range []interface{}{&entity.User{}, &entity.UserTenant{}} {
		if err := db.AutoMigrate(model); err != nil {
			t.Fatalf("migrate %T: %v", model, err)
		}
	}

	now := time.Now().Local().Truncate(time.Second)
	user := &entity.User{
		ID:              "member0000000000000000000000001",
		Nickname:        "member",
		Email:           "member@example.invalid",
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
	}
	user.UpdateDate = &now
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	membership := &entity.UserTenant{
		ID:       "membership000000000000000000001",
		UserID:   user.ID,
		TenantID: "tenant0000000000000000000000001",
		Role:     "member",
		Status:   sqliteTestPtr("1"),
	}
	if err := db.Create(membership).Error; err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	members, err := NewUserTenantDAO().GetMembersByTenantID(ctx, db, membership.TenantID)
	if err != nil {
		t.Fatalf("GetMembersByTenantID: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("members = %d, want 1", len(members))
	}
	if members[0].Nickname != "member" {
		t.Fatalf("nickname = %q", members[0].Nickname)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`).MatchString(members[0].UpdateDate) {
		t.Fatalf("update_date = %q, want YYYY-MM-DDTHH:MM:SS", members[0].UpdateDate)
	}
}

// Canvas tag filtering is written as `tags REGEXP ?`, which SQLite only
// accepts once the callback is registered.
func TestSQLiteRegexpOperator(t *testing.T) {
	// Registration has to precede the first connection: the driver copies the
	// function table into each connection as it opens.
	if err := registerSQLiteRegexp(); err != nil {
		t.Fatalf("register regexp: %v", err)
	}
	db := newSQLiteTestDB(t)

	const pattern = `(^|,)[[:space:]]*go[[:space:]]*(,|$)`
	for value, want := range map[string]int64{
		"go":          1,
		"a, go ,b":    1,
		"golang":      0,
		"python, gox": 0,
		"":            0,
	} {
		var got int64
		if err := db.Raw("SELECT ? REGEXP ?", value, pattern).Scan(&got).Error; err != nil {
			t.Fatalf("regexp(%q): %v", value, err)
		}
		if got != want {
			t.Errorf("regexp(%q) = %d, want %d", value, got, want)
		}
	}
}
