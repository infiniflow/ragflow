package dao

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDatasetFolderBindingMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if err := db.Exec("CREATE TABLE knowledgebase (id varchar(32) PRIMARY KEY, tenant_id varchar(32) NOT NULL, name varchar(128) NOT NULL, embd_id varchar(128) NOT NULL, created_by varchar(32) NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO knowledgebase (id, tenant_id, name, embd_id, created_by) VALUES ('legacy', 'tenant', 'existing dataset', '', 'tenant')").Error; err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn(&entity.Knowledgebase{}, "folder_id") {
		t.Fatal("fixture unexpectedly includes binding column")
	}
	if err := autoMigrateSafely(context.Background(), db, &entity.Knowledgebase{}); err != nil {
		t.Fatal(err)
	}
	var kb entity.Knowledgebase
	if err := db.Where("id = ?", "legacy").Take(&kb).Error; err != nil {
		t.Fatal(err)
	}
	if kb.Name != "existing dataset" || kb.FolderID != nil {
		t.Fatalf("migration changed old data: name=%s binding=%v", kb.Name, kb.FolderID)
	}
	columns, err := db.Migrator().ColumnTypes(&entity.Knowledgebase{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, column := range columns {
		if column.Name() == "folder_id" {
			found = true
			nullable, ok := column.Nullable()
			if !ok || !nullable {
				t.Fatal("binding column is not nullable")
			}
		}
	}
	if !found {
		t.Fatal("binding column missing")
	}
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "legacy").Update("folder_id", "known-folder").Error; err != nil {
		t.Fatal(err)
	}
	if err := autoMigrateSafely(context.Background(), db, &entity.Knowledgebase{}); err != nil {
		t.Fatal(err)
	}
	var after entity.Knowledgebase
	if err := db.Where("id = ?", "legacy").Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.FolderID == nil || *after.FolderID != "known-folder" {
		t.Fatal("repeat migration discarded binding")
	}
}
func TestDatasetFolderBindingDialectShapes(t *testing.T) {
	for _, name := range []string{"mysql", "postgres"} {
		t.Run(name, func(t *testing.T) {
			var dialect gorm.Dialector
			if name == "mysql" {
				dialect = mysql.New(mysql.Config{DSN: "unused:unused@tcp(127.0.0.1:1)/unused", SkipInitializeWithVersion: true})
			} else {
				dialect = postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable"})
			}
			db, err := gorm.Open(dialect, &gorm.Config{DisableAutomaticPing: true, DryRun: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			statement := &gorm.Statement{DB: db}
			if err := statement.Parse(&entity.Knowledgebase{}); err != nil {
				t.Fatal(err)
			}
			field := statement.Schema.LookUpField("folder_id")
			if field == nil || field.NotNull {
				t.Fatal("nullable binding field missing")
			}
			sqlType := db.Migrator().FullDataTypeOf(field).SQL
			if !strings.Contains(strings.ToLower(sqlType), "varchar(32)") {
				t.Fatalf("unexpected binding SQL type %q", sqlType)
			}
			query := db.Model(&entity.Knowledgebase{}).Where("id = ? AND tenant_id = ?", "dataset-uuid", "tenant-uuid").Where("folder_id IS NULL OR folder_id = ''").Update("folder_id", "folder-uuid")
			if query.Error != nil {
				t.Fatal(query.Error)
			}
			t.Logf("dialect=%s nullable type=%s SQL=%s vars=%v", name, sqlType, query.Statement.SQL.String(), query.Statement.Vars)
		})
	}
}
