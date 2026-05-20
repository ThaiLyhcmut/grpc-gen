package logger

import (
	"runtime"
	"strings"
)

// GetCallerFunctionName gets the actual function name of the caller (skips n frames)
func GetCallerFunctionName(skip int) string {
	pc, _, _, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "unknown"
	}
	fullName := fn.Name()

	// Extract just the function name from full path
	// e.g. "github.com/user/project/service/user.(*Handler).CreateStudent" -> "CreateStudent"
	parts := strings.Split(fullName, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return fullName
}
