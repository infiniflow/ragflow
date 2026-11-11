package dao

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

func TestSQLLoggerDoesNotExposeParameters(t *testing.T) {
	var output bytes.Buffer
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: sqlLogger(log.New(&output, "", 0), gormLogger.Info)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("SELECT ?", "platform-secret").Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "SELECT") || strings.Contains(output.String(), "platform-secret") {
		t.Fatalf("unexpected SQL log: %s", output.String())
	}
}
