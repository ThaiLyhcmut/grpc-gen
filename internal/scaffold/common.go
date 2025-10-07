package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func createCommonProto() error {
	// Read module path
	data, _ := os.ReadFile("go.mod")
	modulePath := "mymodule"
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			modulePath = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			break
		}
	}

	content := fmt.Sprintf(`syntax = "proto3";

package common;

option go_package = "%s/proto/common";

// Pagination request
message PaginationRequest {
  int32 page = 1;
  int32 page_size = 2;
  string sort_by = 3;
  bool descending = 4;
}

// Filter condition
message FilterCondition {
  string field = 1;
  string operator = 2; // eq, ne, gt, gte, lt, lte, like, in
  string value = 3;
}

// Filter (can be condition or group)
message Filter {
  oneof filter {
    FilterCondition condition = 1;
  }
}

// Search request with pagination and filters
message SearchRequest {
  PaginationRequest pagination = 1;
  repeated Filter filters = 2;
}
`, modulePath)

	return os.WriteFile(filepath.Join("proto", "common", "common.proto"), []byte(content), 0644)
}
