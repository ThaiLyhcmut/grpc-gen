# Filter Migration Guide - Supporting Nested Group Filters

## Summary

The filter system has been upgraded to support **nested group filters** with `AND`/`OR` logic operators.

**Previous limitation:** Only simple conditions were supported
**New capability:** Full nested filter groups with recursive logic

## Test Results

All tests pass ✅:
```bash
go test -v ./pkg/helper -run TestBuild
go test -v ./pkg/helper -run TestYourExact
go test -v ./pkg/helper -run TestFilterIntegration
```

### Example: Your JSON Request

**Input JSON:**
```json
{
  "search": {
    "filters": [
      {
        "condition": {
          "field": "title",
          "operator": "LIKE",
          "values": ["10"]
        }
      },
      {
        "group": {
          "logic": "OR",
          "filters": [
            {
              "condition": {
                "field": "title",
                "operator": "LIKE",
                "values": ["CNTT"]
              }
            }
          ]
        }
      }
    ],
    "pagination": {
      "descending": false,
      "page": 1,
      "page_size": 100,
      "sort_by": "created_at"
    }
  }
}
```

**Generated SQL:**
```sql
SELECT * FROM Topic
WHERE title LIKE ? AND title LIKE ?
ORDER BY created_at ASC
LIMIT 100 OFFSET 0
```

**Args:** `["%10%", "%CNTT%"]`

---

## Migration Steps

### Step 1: Update Handler Code

**OLD CODE** (only handles conditions):
```go
if req.Search != nil && len(req.Search.Filters) > 0 {
    whereConditions := []string{}
    for _, filter := range req.Search.Filters {
        if filter.GetCondition() != nil {
            condition := filter.GetCondition()
            if _, ok := whiteMap[condition.Field]; !ok {
                continue
            }
            whereConditions = append(whereConditions, helper.BuildFilterCondition(condition, &args))
        }
    }
    if len(whereConditions) > 0 {
        whereClause = "WHERE " + strings.Join(whereConditions, " AND ")
    }
}
```

**NEW CODE** (handles both conditions and groups):
```go
if req.Search != nil && len(req.Search.Filters) > 0 {
    whereConditions := []string{}
    for _, filter := range req.Search.Filters {
        condition := helper.BuildFilterCriteriaWithWhitelist(filter, &args, whiteMap)
        if condition != "" && condition != "1=1" {
            whereConditions = append(whereConditions, condition)
        }
    }
    if len(whereConditions) > 0 {
        whereClause = "WHERE " + strings.Join(whereConditions, " AND ")
    }
}
```

### Step 2: Files to Update

Run this command to find all handlers that need updating:
```bash
grep -r "BuildFilterCondition" --include="*.go" thesis/handler/ academic/handler/ council/handler/ user/handler/ file/handler/ role/handler/
```

Expected files:
- `thesis/handler/topic.go`
- `thesis/handler/enrollment.go`
- `thesis/handler/midterm.go`
- `thesis/handler/final.go`
- `thesis/handler/topiccouncil.go`
- `thesis/handler/topiccouncilsupervisor.go`
- `thesis/handler/gradereview.go`
- `academic/handler/major.go`
- `academic/handler/faculty.go`
- `academic/handler/semester.go`
- `council/handler/council.go`
- `council/handler/gradedefence.go`
- `council/handler/gradedefencecriterion.go`
- `council/handler/defence.go`
- `user/handler/teacher.go`
- `user/handler/student.go`
- `file/handler/file.go`
- `role/handler/rolesystem.go`

---

## New Helper Functions

### 1. `BuildFilterCriteria(criteria, args)`
Handles both simple conditions and nested groups (no field validation).

**Use when:** You trust all input fields (e.g., internal system calls).

### 2. `BuildFilterCriteriaWithWhitelist(criteria, args, whiteMap)`
Handles both simple conditions and nested groups **with field validation**.

**Use when:** Processing user input (recommended for all handlers).

### 3. `BuildFilterGroup(group, args)`
Recursively builds SQL for filter groups.

### 4. `BuildFilterGroupWithWhitelist(group, args, whiteMap)`
Recursively builds SQL for filter groups with field validation.

---

## Complex Example

**Input:**
```json
{
  "filters": [
    {"condition": {"field": "status", "operator": "EQUAL", "values": ["active"]}},
    {
      "group": {
        "logic": "OR",
        "filters": [
          {"condition": {"field": "major_code", "operator": "IN", "values": ["CNTT", "KTPM"]}},
          {
            "group": {
              "logic": "AND",
              "filters": [
                {"condition": {"field": "semester_code", "operator": "EQUAL", "values": ["HK1-2024"]}},
                {"condition": {"field": "percent_stage_1", "operator": "GREATER_THAN", "values": ["70"]}}
              ]
            }
          }
        ]
      }
    }
  ]
}
```

**Generated SQL:**
```sql
WHERE status = ?
  AND (major_code IN (?, ?) OR (semester_code = ? AND percent_stage_1 > ?))
```

**Args:**
```go
["active", "CNTT", "KTPM", "HK1-2024", "70"]
```

---

## Security

✅ **Field Whitelist Validation:** Invalid fields are automatically filtered out
✅ **SQL Injection Protection:** All values use parameterized queries
✅ **Recursive Validation:** Nested groups are validated recursively

**Test with malicious input:**
```go
// Input includes invalid fields "hacker_field" and "another_invalid"
// Output: Only valid whitelisted fields are included in SQL
WHERE title LIKE ? AND major_code = ?
```

---

## Testing

Run all filter tests:
```bash
cd /home/thaily/code/heheheh_be/src/service
go test -v ./pkg/helper -run TestFilter
```

Expected output:
```
✅ TestBuildFilterCondition
✅ TestBuildNestedFilters
✅ TestBuildNestedFiltersWithProperImplementation
✅ TestYourExactJSONRequest
✅ TestComplexNestedFilters
✅ TestFilterIntegrationWithWhitelist
✅ TestFilterIntegrationWithInvalidField
✅ TestFilterIntegrationComplexNested
```

---

## Summary of Changes

| File | Function Added | Description |
|------|---------------|-------------|
| `pkg/helper/filter.go` | `BuildFilterGroup()` | Recursively build SQL from FilterGroup |
| `pkg/helper/filter.go` | `BuildFilterCriteria()` | Build SQL from FilterCriteria (condition or group) |
| `pkg/helper/filter.go` | `BuildFilterCriteriaWithWhitelist()` | Build SQL with field validation |
| `pkg/helper/filter.go` | `BuildFilterGroupWithWhitelist()` | Build SQL from group with field validation |

---

## Next Steps

1. ✅ Test filter functions (DONE)
2. ⏳ Update handler files to use `BuildFilterCriteriaWithWhitelist()`
3. ⏳ Run integration tests with real database
4. ⏳ Test with frontend/API client

---

## Questions?

If you encounter issues:
1. Check test output: `go test -v ./pkg/helper`
2. Review generated SQL in test logs
3. Verify whitelist map includes all required fields
4. Check proto definition: `/home/thaily/code/heheheh_be/proto/common/common.proto`
