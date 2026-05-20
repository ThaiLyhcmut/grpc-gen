package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc"
)

// UnaryServerInterceptor creates a gRPC interceptor that adds tracing to all requests
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Add request ID and trace stack to context
		ctx = WithRequestID(ctx)
		ctx = WithTraceStack(ctx)

		// Record start time
		start := time.Now()

		// Call the handler
		resp, err := handler(ctx, req)

		// Calculate duration
		duration := time.Since(start)

		// Get trace information
		stack := GetTraceStack(ctx)
		var trace *FunctionTrace
		if stack != nil {
			trace = stack.GetRoot()
		}

		// Print trace as JSON
		traceData := map[string]interface{}{
			"request_id":  GetRequestID(ctx),
			"method":      info.FullMethod,
			"duration_ms": duration.Milliseconds(),
			"success":     err == nil,
		}

		if trace != nil {
			traceData["trace"] = trace
			traceData["queries"] = FlattenQueries(trace)
		}

		if err != nil {
			traceData["error"] = err.Error()
		}

		// Write trace to file logger
		fileLogger := GetFileLogger()
		if fileLogger != nil {
			fileLogger.WriteTrace(traceData)
		} else {
			// Fallback to console if file logger not initialized
			jsonData, _ := json.MarshalIndent(traceData, "", "  ")
			fmt.Printf("\n=== Request Trace ===\n%s\n====================\n\n", string(jsonData))
		}

		return resp, err
	}
}
