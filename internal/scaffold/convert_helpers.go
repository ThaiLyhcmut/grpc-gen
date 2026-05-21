package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
)

// generateConvertHelpers generates helper functions for convert package
func generateConvertHelpers(serverDir string, modulePath string) error {
	content := `package convert

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// pbTimestampToTime converts protobuf Timestamp to *time.Time
func pbTimestampToTime(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

// timeToPbTimestamp converts *time.Time to protobuf Timestamp
func timeToPbTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// intPtr returns a pointer to int
func intPtr(i int) *int {
	return &i
}

// int32Ptr returns a pointer to int32
func int32Ptr(i int32) *int32 {
	return &i
}

// int64Ptr returns a pointer to int64
func int64Ptr(i int64) *int64 {
	return &i
}

// stringPtr returns a pointer to string
func stringPtr(s string) *string {
	return &s
}

// boolPtr returns a pointer to bool
func boolPtr(b bool) *bool {
	return &b
}

// derefString returns the value of a string pointer or empty string
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// derefInt returns the value of an int pointer or 0
func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

// derefInt32 returns the value of an int32 pointer or 0
func derefInt32(i *int32) int32 {
	if i == nil {
		return 0
	}
	return *i
}

// derefBool returns the value of a bool pointer or false
func derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
`
	helpersFile := filepath.Join(serverDir, "graph", "convert", "helpers.go")
	return os.WriteFile(helpersFile, []byte(content), 0644)
}

// Update generateConvertFunctions to also generate helpers
func generateConvertFunctionsWithHelpers(serverDir string, protos []ProtoInfo, modulePath string) error {
	// First generate helpers
	if err := generateConvertHelpers(serverDir, modulePath); err != nil {
		return fmt.Errorf("failed to generate convert helpers: %w", err)
	}

	// Then generate convert functions for each proto
	for _, proto := range protos {
		if len(proto.Messages) == 0 {
			continue
		}

		if err := generateConvertFile(serverDir, proto, modulePath); err != nil {
			return err
		}
	}
	return nil
}
