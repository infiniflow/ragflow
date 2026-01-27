package dao

import (
	"fmt"
	"time"

	gormLogger "gorm.io/gorm/logger"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"ragflow/internal/config"
	"ragflow/internal/logger"
)

var DB *gorm.DB

// InitDB initialize database connection
func InitDB() error {
	cfg := config.Get()
	dbCfg := cfg.Database

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		dbCfg.Username,
		dbCfg.Password,
		dbCfg.Host,
		dbCfg.Port,
		dbCfg.Database,
		dbCfg.Charset,
	)

	// Set log level
	var gormLogLevel gormLogger.LogLevel
	if cfg.Server.Mode == "debug" {
		gormLogLevel = gormLogger.Info
	} else {
		gormLogLevel = gormLogger.Silent
	}

	// Connect to database
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormLogger.Default.LogMode(gormLogLevel),
		NowFunc: func() time.Time {
			return time.Now().Local()
		},
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	// Get general database object sql.DB
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// Set connection pool
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Auto migrate
	//if err := DB.AutoMigrate(&model.User{}, &model.Document{}); err != nil {
	//	return fmt.Errorf("failed to migrate database: %w", err)
	//}

	logger.Info("Database connected and migrated successfully")
	return nil
}

// GetDB get database instance
func GetDB() *gorm.DB {
	return DB
}
