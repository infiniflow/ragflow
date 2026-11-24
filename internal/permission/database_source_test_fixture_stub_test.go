//go:build !ee

package permission

import "gorm.io/gorm"

func newEditionDatabaseChecker(db *gorm.DB) *Checker { return newCommunityDatabaseChecker(db) }

func prepareEnterpriseDatabaseSourceFixture(*gorm.DB, string) error { return nil }
