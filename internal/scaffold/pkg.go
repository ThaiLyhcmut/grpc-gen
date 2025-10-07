package scaffold

import (
	
	"os"
	"path/filepath"
)

func createPkgFiles(modulePath string) error {
	// Create database.go
	if err := createDatabaseFile(modulePath); err != nil {
		return err
	}

	// Create logger files
	if err := createLoggerFiles(modulePath); err != nil {
		return err
	}

	// Create helper files
	if err := createHelperFiles(modulePath); err != nil {
		return err
	}

	return nil
}

func createDatabaseFile(modulePath string) error {
	content := `package database

import (
	"database/sql"
	
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var DB *sql.DB

func InitDB() error {
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		dbUser, dbPassword, dbHost, dbPort, dbName)

	var err error
	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)
	DB.SetConnMaxLifetime(5 * time.Minute)

	if err = DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Database connected successfully")
	return nil
}

func CloseDB() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
`

	return os.WriteFile(filepath.Join("src", "pkg", "database", "database.go"), []byte(content), 0644)
}

func createLoggerFiles(modulePath string) error {
	// Create simple logger.go
	content := `package logger

import (
	"context"
	
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"google.golang.org/grpc"
)

var fileLogger *os.File

func InitFileLogger(serviceName, logDir string) error {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	logFile := filepath.Join(logDir, fmt.Sprintf("%s_%s.log",
		serviceName, time.Now().Format("2006-01-02")))

	var err error
	fileLogger, err = os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	log.SetOutput(fileLogger)
	return nil
}

func GetFileLogger() *os.File {
	return fileLogger
}

func TraceFunction(ctx context.Context) func() {
	pc, _, _, _ := runtime.Caller(1)
	funcName := runtime.FuncForPC(pc).Name()
	start := time.Now()

	log.Printf("[TRACE] Entering %s", funcName)

	return func() {
		log.Printf("[TRACE] Exiting %s (duration: %v)", funcName, time.Since(start))
	}
}

func UnaryServerInterceptor() grpc.ServerOption {
	return grpc.UnaryInterceptor(func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		log.Printf("[RPC] %s started", info.FullMethod)

		resp, err := handler(ctx, req)

		if err != nil {
			log.Printf("[RPC] %s failed: %v (duration: %v)",
				info.FullMethod, err, time.Since(start))
		} else {
			log.Printf("[RPC] %s completed (duration: %v)",
				info.FullMethod, time.Since(start))
		}

		return resp, err
	})
}
`

	return os.WriteFile(filepath.Join("src", "pkg", "logger", "logger.go"), []byte(content), 0644)
}

func createHelperFiles(modulePath string) error {
	content := `package helper

import (
	pb "` + modulePath + `/proto/common"
	
	"strings"
)

func BuildFilterCondition(condition *pb.FilterCondition, args *[]interface{}) string {
	switch condition.Operator {
	case "eq":
		*args = append(*args, condition.Value)
		return fmt.Sprintf("%s = ?", condition.Field)
	case "ne":
		*args = append(*args, condition.Value)
		return fmt.Sprintf("%s != ?", condition.Field)
	case "gt":
		*args = append(*args, condition.Value)
		return fmt.Sprintf("%s > ?", condition.Field)
	case "gte":
		*args = append(*args, condition.Value)
		return fmt.Sprintf("%s >= ?", condition.Field)
	case "lt":
		*args = append(*args, condition.Value)
		return fmt.Sprintf("%s < ?", condition.Field)
	case "lte":
		*args = append(*args, condition.Value)
		return fmt.Sprintf("%s <= ?", condition.Field)
	case "like":
		*args = append(*args, "%"+condition.Value+"%")
		return fmt.Sprintf("%s LIKE ?", condition.Field)
	case "in":
		values := strings.Split(condition.Value, ",")
		placeholders := make([]string, len(values))
		for i, v := range values {
			*args = append(*args, strings.TrimSpace(v))
			placeholders[i] = "?"
		}
		return fmt.Sprintf("%s IN (%s)", condition.Field, strings.Join(placeholders, ","))
	default:
		*args = append(*args, condition.Value)
		return fmt.Sprintf("%s = ?", condition.Field)
	}
}
`

	return os.WriteFile(filepath.Join("src", "pkg", "helper", "filter.go"), []byte(content), 0644)
}
