package logger

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
