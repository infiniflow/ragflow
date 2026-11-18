package dao

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
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
