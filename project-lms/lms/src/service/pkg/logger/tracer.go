package logger

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"time"
)

type traceContextKey string

const (
	traceKey traceContextKey = "trace"
)

// QueryLog represents a single database query
type QueryLog struct {
	Query    string `json:"query"`
	Duration int64  `json:"duration_ms"`
}

// FunctionTrace represents a single function execution
type FunctionTrace struct {
	FunctionName string          `json:"function_name"`
	StartTime    time.Time       `json:"-"`
	EndTime      time.Time       `json:"-"`
	Duration     int64           `json:"duration_ms"`
	Queries      []QueryLog      `json:"queries,omitempty"`
	Children     []FunctionTrace `json:"children,omitempty"`
}

// TraceStack manages the call stack trace
type TraceStack struct {
	mu    sync.Mutex
	stack []*FunctionTrace
	root  *FunctionTrace
}

// NewTraceStack creates a new trace stack
func NewTraceStack() *TraceStack {
	return &TraceStack{
		stack: make([]*FunctionTrace, 0),
	}
}

// Push adds a new function to the trace stack
func (ts *TraceStack) Push(functionName string) *FunctionTrace {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	trace := &FunctionTrace{
		FunctionName: functionName,
		StartTime:    time.Now(),
		Children:     make([]FunctionTrace, 0),
		Queries:      make([]QueryLog, 0),
	}

	if len(ts.stack) == 0 {
		// This is the root
		ts.root = trace
	} else {
		// Add as child of current top
		parent := ts.stack[len(ts.stack)-1]
		parent.Children = append(parent.Children, *trace)
	}

	ts.stack = append(ts.stack, trace)
	return trace
}

// Pop removes the top function from stack
func (ts *TraceStack) Pop() *FunctionTrace {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if len(ts.stack) == 0 {
		return nil
	}

	trace := ts.stack[len(ts.stack)-1]
	trace.EndTime = time.Now()
	trace.Duration = trace.EndTime.Sub(trace.StartTime).Milliseconds()

	ts.stack = ts.stack[:len(ts.stack)-1]

	return trace
}

// GetRoot returns the root trace
func (ts *TraceStack) GetRoot() *FunctionTrace {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.root
}

// AddQuery adds a query to the current function
func (ts *TraceStack) AddQuery(query string, durationMs int64) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if len(ts.stack) > 0 {
		current := ts.stack[len(ts.stack)-1]
		current.Queries = append(current.Queries, QueryLog{
			Query:    query,
			Duration: durationMs,
		})
	}
}

// WithTraceStack adds a trace stack to context
func WithTraceStack(ctx context.Context) context.Context {
	return context.WithValue(ctx, traceKey, NewTraceStack())
}

// GetTraceStack gets the trace stack from context
func GetTraceStack(ctx context.Context) *TraceStack {
	if stack, ok := ctx.Value(traceKey).(*TraceStack); ok {
		return stack
	}
	return nil
}

// TraceFunction automatically traces a function execution
// Usage: defer logger.TraceFunction(ctx)()
func TraceFunction(ctx context.Context) func() {
	stack := GetTraceStack(ctx)
	if stack == nil {
		return func() {}
	}

	// Get caller function name
	pc, _, _, ok := runtime.Caller(1)
	if !ok {
		return func() {}
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return func() {}
	}

	functionName := extractFunctionName(fn.Name())
	stack.Push(functionName)

	return func() {
		stack.Pop()
	}
}

// TraceFunctionWithName traces with explicit name
func TraceFunctionWithName(ctx context.Context, name string) func() {
	stack := GetTraceStack(ctx)
	if stack == nil {
		return func() {}
	}

	stack.Push(name)

	return func() {
		stack.Pop()
	}
}

// AddQueryToTrace adds a query to current trace
func AddQueryToTrace(ctx context.Context, query string, durationMs int64) {
	stack := GetTraceStack(ctx)
	if stack != nil {
		stack.AddQuery(query, durationMs)
	}
}

// extractFunctionName extracts clean function name
func extractFunctionName(fullName string) string {
	// Remove package path
	parts := strings.Split(fullName, "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]

		// Remove receiver type info like "(*Handler)."
		name = strings.ReplaceAll(name, "(*", "")
		name = strings.ReplaceAll(name, ")", "")
		name = strings.ReplaceAll(name, "*", "")

		// Get last part after dot
		dotParts := strings.Split(name, ".")
		if len(dotParts) > 0 {
			return dotParts[len(dotParts)-1]
		}
		return name
	}
	return fullName
}

// GetAllTraces gets all function traces in flat format
func GetAllTraces(trace *FunctionTrace) []FunctionTrace {
	if trace == nil {
		return nil
	}

	result := []FunctionTrace{*trace}
	for _, child := range trace.Children {
		result = append(result, GetAllTraces(&child)...)
	}
	return result
}

// FlattenQueries gets all queries from all traces
func FlattenQueries(trace *FunctionTrace) []QueryLog {
	if trace == nil {
		return nil
	}

	queries := make([]QueryLog, 0)
	queries = append(queries, trace.Queries...)

	for _, child := range trace.Children {
		queries = append(queries, FlattenQueries(&child)...)
	}

	return queries
}
