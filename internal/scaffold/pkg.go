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
	"fmt"

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
	"fmt"

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
	"fmt"
)


// BuildFilterCondition builds SQL WHERE condition from FilterCondition (MySQL syntax with ?)
func BuildFilterCondition(condition *pbCommon.FilterCondition, args *[]interface{}) string {
	field := condition.Field
	operator := condition.Operator
	values := condition.Values

	switch operator {
	case pbCommon.FilterOperator_EQUAL:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s = ?", field)
	case pbCommon.FilterOperator_NOT_EQUAL:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s != ?", field)
	case pbCommon.FilterOperator_GREATER_THAN:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s > ?", field)
	case pbCommon.FilterOperator_GREATER_THAN_EQUAL:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s >= ?", field)
	case pbCommon.FilterOperator_LESS_THAN:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s < ?", field)
	case pbCommon.FilterOperator_LESS_THAN_EQUAL:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s <= ?", field)
	case pbCommon.FilterOperator_LIKE:
		*args = append(*args, "%"+values[0]+"%")
		return fmt.Sprintf("%s LIKE ?", field)
	case pbCommon.FilterOperator_IN:
		placeholders := []string{}
		for _, val := range values {
			*args = append(*args, val)
			placeholders = append(placeholders, "?")
		}
		return fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ", "))
	case pbCommon.FilterOperator_NOT_IN:
		placeholders := []string{}
		for _, val := range values {
			*args = append(*args, val)
			placeholders = append(placeholders, "?")
		}
		return fmt.Sprintf("%s NOT IN (%s)", field, strings.Join(placeholders, ", "))
	case pbCommon.FilterOperator_IS_NULL:
		return fmt.Sprintf("%s IS NULL", field)
	case pbCommon.FilterOperator_IS_NOT_NULL:
		return fmt.Sprintf("%s IS NOT NULL", field)
	case pbCommon.FilterOperator_BETWEEN:
		if len(values) >= 2 {
			*args = append(*args, values[0], values[1])
			return fmt.Sprintf("%s BETWEEN ? AND ?", field)
		}
	}

	return "1=1" // fallback
}

`

	return os.WriteFile(filepath.Join("src", "pkg", "helper", "filter.go"), []byte(content), 0644)
}
