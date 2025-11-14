package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileLogger handles writing logs to files
type FileLogger struct {
	serviceName string
	logDir      string
	file        *os.File
	mu          sync.Mutex
}

var (
	globalFileLogger *FileLogger
	loggerOnce       sync.Once
)

// InitFileLogger initializes the global file logger
func InitFileLogger(serviceName, logDir string) error {
	var err error
	loggerOnce.Do(func() {
		globalFileLogger, err = NewFileLogger(serviceName, logDir)
	})
	return err
}

// GetFileLogger returns the global logger instance
func GetFileLogger() *FileLogger {
	return globalFileLogger
}

// NewFileLogger creates a new file logger instance
func NewFileLogger(serviceName, logDir string) (*FileLogger, error) {
	// Create log directory if not exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logger := &FileLogger{
		serviceName: serviceName,
		logDir:      logDir,
	}

	if err := logger.openLogFile(); err != nil {
		return nil, err
	}

	// Start daily rotation
	go logger.rotateDaily()

	return logger, nil
}

// openLogFile opens or creates a log file for today
func (l *FileLogger) openLogFile() error {
	today := time.Now().Format("2006-01-02")
	filename := filepath.Join(l.logDir, fmt.Sprintf("%s-%s.json", today, l.serviceName))

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.mu.Lock()
	if l.file != nil {
		l.file.Close()
	}
	l.file = file
	l.mu.Unlock()

	return nil
}

// rotateDaily rotates log file daily
func (l *FileLogger) rotateDaily() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	lastDate := time.Now().Format("2006-01-02")

	for range ticker.C {
		currentDate := time.Now().Format("2006-01-02")
		if currentDate != lastDate {
			l.openLogFile()
			lastDate = currentDate
		}
	}
}

// WriteTrace writes a trace log entry
func (l *FileLogger) WriteTrace(data map[string]interface{}) error {
	if l == nil {
		// If logger not initialized, just print to stdout
		jsonData, _ := json.Marshal(data)
		fmt.Println(string(jsonData))
		return nil
	}

	// Add timestamp and service name
	data["timestamp"] = time.Now().Format(time.RFC3339)
	data["service_name"] = l.serviceName

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		l.file.Write(jsonData)
		l.file.Write([]byte("\n"))
	}

	return nil
}

// Close closes the log file
func (l *FileLogger) Close() error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
