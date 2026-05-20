package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var (
	globalDB *sql.DB
	dbMutex  sync.RWMutex
)

// ConnectionPoolConfig chứa các cấu hình cho connection pool
type ConnectionPoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DefaultConnectionPoolConfig trả về config mặc định
// Với 6 services cùng dùng 1 DB (max_connections=151), mỗi service dùng tối đa 20-25 connections
func DefaultConnectionPoolConfig() ConnectionPoolConfig {
	return ConnectionPoolConfig{
		MaxOpenConns:    20, // Giảm xuống 20 để an toàn hơn (6 services * 20 = 120 < 151)
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}
}

// LoadConnectionPoolConfigFromEnv load config từ environment variables
func LoadConnectionPoolConfigFromEnv() ConnectionPoolConfig {
	config := DefaultConnectionPoolConfig()

	if val := os.Getenv("DB_MAX_OPEN_CONNS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			config.MaxOpenConns = n
		}
	}

	if val := os.Getenv("DB_MAX_IDLE_CONNS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			config.MaxIdleConns = n
		}
	}

	if val := os.Getenv("DB_CONN_MAX_LIFETIME"); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			config.ConnMaxLifetime = d
		}
	}

	if val := os.Getenv("DB_CONN_MAX_IDLE_TIME"); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			config.ConnMaxIdleTime = d
		}
	}

	return config
}

func Connect(dsn string) (*sql.DB, error) {
	return ConnectWithConfig(dsn, LoadConnectionPoolConfigFromEnv())
}

func ConnectWithConfig(dsn string, config ConnectionPoolConfig) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Cấu hình Connection Pool để tối ưu và tránh "too many connections"

	// Số lượng connection tối đa có thể mở (bao gồm cả đang dùng và idle)
	// Nên set thấp hơn max_connections của MySQL (thường là 151)
	// Nếu có nhiều services cùng dùng 1 DB, chia đều số connection
	db.SetMaxOpenConns(config.MaxOpenConns)

	// Số lượng connection idle tối đa được giữ lại trong pool
	// Giúp tái sử dụng connection thay vì tạo mới liên tục
	db.SetMaxIdleConns(config.MaxIdleConns)

	// Thời gian tối đa 1 connection có thể được sử dụng (connection lifetime)
	// Sau thời gian này connection sẽ bị đóng và tạo mới
	// Tránh connection bị stale hoặc MySQL timeout
	db.SetConnMaxLifetime(config.ConnMaxLifetime)

	// Thời gian tối đa 1 connection idle được giữ trong pool
	// Giúp giải phóng connection không dùng đến
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Database connected successfully with connection pool:")
	log.Printf("  - MaxOpenConns: %d", config.MaxOpenConns)
	log.Printf("  - MaxIdleConns: %d", config.MaxIdleConns)
	log.Printf("  - ConnMaxLifetime: %v", config.ConnMaxLifetime)
	log.Printf("  - ConnMaxIdleTime: %v", config.ConnMaxIdleTime)

	return db, nil
}

// InitDB initializes the global database connection from environment variables
func InitDB() error {
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	if dbHost == "" || dbPort == "" || dbUser == "" || dbName == "" {
		return fmt.Errorf("missing required database environment variables")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Local",
		dbUser, dbPassword, dbHost, dbPort, dbName)

	db, err := Connect(dsn)
	if err != nil {
		return err
	}

	dbMutex.Lock()
	globalDB = db
	dbMutex.Unlock()

	return nil
}

// GetDB returns the global database connection
func GetDB() *sql.DB {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	return globalDB
}

// CloseDB closes the global database connection
func CloseDB() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if globalDB != nil {
		err := globalDB.Close()
		globalDB = nil
		return err
	}
	return nil
}
