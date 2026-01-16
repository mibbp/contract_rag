package postgres

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB 初始化 PG 连接
// dsn 格式: "host=localhost user=postgres password=root dbname=mydb port=5432 sslmode=disable"
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info), // 开发阶段开启日志，方便看 SQL
	})
	if err != nil {
		return nil, fmt.Errorf("connect db failed: %w", err)
	}

	// 设置连接池（生产环境必备）
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(10)           // 空闲连接数
	sqlDB.SetMaxOpenConns(100)          // 最大连接数
	sqlDB.SetConnMaxLifetime(time.Hour) // 连接最大复用时间

	log.Println("PostgreSQL connected successfully")
	return db, nil
}
