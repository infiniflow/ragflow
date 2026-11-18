package dao

import (
	"database/sql"
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// OpenIndependentMySQLDB opens a separate pool with the active connection
// settings. Session locks must not reserve connections from the application
// pool while their owners run database operations through that same pool.
func OpenIndependentMySQLDB(db *gorm.DB) (*sql.DB, error) {
	var config *mysql.Config
	switch dialector := db.Dialector.(type) {
	case migrationAwareDialector:
		config = dialector.Config
	case *mysql.Dialector:
		config = dialector.Config
	default:
		return nil, fmt.Errorf("independent session requires a MySQL dialector")
	}
	if config == nil || config.DSN == "" {
		return nil, fmt.Errorf("independent session requires a MySQL DSN")
	}
	driverName := config.DriverName
	if driverName == "" {
		driverName = "mysql"
	}
	pool, err := sql.Open(driverName, config.DSN)
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(0)
	return pool, nil
}
