package dao

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestIndependentMySQLDBSupportsMigrationDialector(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		name := "mysql"
		if wrapped {
			name = "migration wrapper"
		}
		t.Run(name, func(t *testing.T) {
			shared, _, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { shared.Close() })
			base := mysql.New(mysql.Config{DSN: "test:test@tcp(localhost:3306)/test?parseTime=True", Conn: shared, SkipInitializeWithVersion: true}).(*mysql.Dialector)
			var dialect gorm.Dialector = base
			if wrapped {
				dialect = migrationAwareDialector{Dialector: base}
			}
			db, err := gorm.Open(dialect, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			independent, err := OpenIndependentMySQLDB(db)
			if err != nil {
				t.Fatal(err)
			}
			defer independent.Close()
			if independent == shared || independent.Stats().MaxOpenConnections != 1 {
				t.Fatal("session lock must have its own bounded connection pool")
			}
		})
	}
}

func TestShareModeLocksForOceanBase(t *testing.T) {
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	// OceanBase can report an 8.x version, which alone selects FOR SHARE.
	mock.ExpectQuery("SELECT VERSION()").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("8.0.30-OceanBase_CE-v4.4.1.0"))
	dialect := migrationAwareDialector{Dialector: mysql.New(mysql.Config{Conn: conn}).(*mysql.Dialector)}
	useShareModeLocks(dialect)
	db, err := gorm.Open(dialect, &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	var kb struct{ ID string }
	stmt := db.Table("knowledgebase").Clauses(clause.Locking{Strength: "SHARE"}).Take(&kb).Statement
	if sql := stmt.SQL.String(); !strings.Contains(sql, "LOCK IN SHARE MODE") || strings.Contains(sql, "FOR SHARE") {
		t.Fatalf("shared lock SQL = %q", sql)
	}
}
