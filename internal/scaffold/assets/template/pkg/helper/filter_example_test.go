package helper

import (
	"encoding/json"
	"strings"
	"testing"
	pbCommon "thaily/proto/common"
)

// TestYourExactJSONRequest tests the exact JSON structure you provided
func TestYourExactJSONRequest(t *testing.T) {
	// Your JSON request:
	// {
	//   "search": {
	//     "filters": [
	//       {"condition": {"field": "title", "operator": "LIKE", "values": ["10"]}},
	//       {"group": {"logic": "OR", "filters": [
	//         {"condition": {"field": "title", "operator": "LIKE", "values": ["CNTT"]}}
	//       ]}}
	//     ],
	//     "pagination": {"descending": false, "page": 1, "page_size": 100, "sort_by": "created_at"}
	//   }
	// }

	filters := []*pbCommon.FilterCriteria{
		{
			Criteria: &pbCommon.FilterCriteria_Condition{
				Condition: &pbCommon.FilterCondition{
					Field:    "title",
					Operator: pbCommon.FilterOperator_LIKE,
					Values:   []string{"10"},
				},
			},
		},
		{
			Criteria: &pbCommon.FilterCriteria_Group{
				Group: &pbCommon.FilterGroup{
					Logic: pbCommon.LogicalCondition_OR,
					Filters: []*pbCommon.FilterCriteria{
						{
							Criteria: &pbCommon.FilterCriteria_Condition{
								Condition: &pbCommon.FilterCondition{
									Field:    "title",
									Operator: pbCommon.FilterOperator_LIKE,
									Values:   []string{"CNTT"},
								},
							},
						},
					},
				},
			},
		},
	}

	// Build SQL conditions
	args := []interface{}{}
	whereConditions := []string{}

	for _, filter := range filters {
		condition := BuildFilterCriteria(filter, &args)
		if condition != "" && condition != "1=1" {
			whereConditions = append(whereConditions, condition)
		}
	}

	whereClause := ""
	if len(whereConditions) > 0 {
		whereClause = "WHERE " + strings.Join(whereConditions, " AND ")
	}

	// Print results
	t.Logf("======================================")
	t.Logf("Generated WHERE clause: %s", whereClause)
	t.Logf("Generated Args: %v", args)
	t.Logf("======================================")
	t.Logf("")

	// Full SQL query example
	fullSQL := "SELECT * FROM Topic " + whereClause + " ORDER BY created_at ASC LIMIT 100 OFFSET 0"
	t.Logf("Full SQL Query:")
	t.Logf("%s", fullSQL)
	t.Logf("")
	t.Logf("With Args: %v", args)
	t.Logf("======================================")

	// Expected results
	expectedWhereClause := "WHERE title LIKE ? AND title LIKE ?"
	expectedArgs := []interface{}{"%10%", "%CNTT%"}

	if whereClause != expectedWhereClause {
		t.Errorf("WHERE clause mismatch:\nExpected: %s\nGot: %s", expectedWhereClause, whereClause)
	}

	if len(args) != len(expectedArgs) {
		t.Errorf("Args length mismatch: expected %d, got %d", len(expectedArgs), len(args))
	}

	for i, arg := range args {
		if arg != expectedArgs[i] {
			t.Errorf("Arg[%d] mismatch: expected %v, got %v", i, expectedArgs[i], arg)
		}
	}
}

// TestComplexNestedFilters tests a more complex nested structure
func TestComplexNestedFilters(t *testing.T) {
	// Complex JSON structure:
	// {
	//   "search": {
	//     "filters": [
	//       {"condition": {"field": "status", "operator": "EQUAL", "values": ["active"]}},
	//       {"group": {"logic": "OR", "filters": [
	//         {"condition": {"field": "major_code", "operator": "IN", "values": ["CNTT", "KTPM"]}},
	//         {"group": {"logic": "AND", "filters": [
	//           {"condition": {"field": "semester_code", "operator": "EQUAL", "values": ["HK1-2024"]}},
	//           {"condition": {"field": "percent_stage_1", "operator": "GREATER_THAN", "values": ["70"]}}
	//         ]}}
	//       ]}}
	//     ]
	//   }
	// }

	filters := []*pbCommon.FilterCriteria{
		{
			Criteria: &pbCommon.FilterCriteria_Condition{
				Condition: &pbCommon.FilterCondition{
					Field:    "status",
					Operator: pbCommon.FilterOperator_EQUAL,
					Values:   []string{"active"},
				},
			},
		},
		{
			Criteria: &pbCommon.FilterCriteria_Group{
				Group: &pbCommon.FilterGroup{
					Logic: pbCommon.LogicalCondition_OR,
					Filters: []*pbCommon.FilterCriteria{
						{
							Criteria: &pbCommon.FilterCriteria_Condition{
								Condition: &pbCommon.FilterCondition{
									Field:    "major_code",
									Operator: pbCommon.FilterOperator_IN,
									Values:   []string{"CNTT", "KTPM"},
								},
							},
						},
						{
							Criteria: &pbCommon.FilterCriteria_Group{
								Group: &pbCommon.FilterGroup{
									Logic: pbCommon.LogicalCondition_AND,
									Filters: []*pbCommon.FilterCriteria{
										{
											Criteria: &pbCommon.FilterCriteria_Condition{
												Condition: &pbCommon.FilterCondition{
													Field:    "semester_code",
													Operator: pbCommon.FilterOperator_EQUAL,
													Values:   []string{"HK1-2024"},
												},
											},
										},
										{
											Criteria: &pbCommon.FilterCriteria_Condition{
												Condition: &pbCommon.FilterCondition{
													Field:    "percent_stage_1",
													Operator: pbCommon.FilterOperator_GREATER_THAN,
													Values:   []string{"70"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	args := []interface{}{}
	whereConditions := []string{}

	for _, filter := range filters {
		condition := BuildFilterCriteria(filter, &args)
		if condition != "" && condition != "1=1" {
			whereConditions = append(whereConditions, condition)
		}
	}

	whereClause := "WHERE " + strings.Join(whereConditions, " AND ")

	t.Logf("======================================")
	t.Logf("Complex Nested Filters Test")
	t.Logf("======================================")
	t.Logf("Generated WHERE clause:")
	t.Logf("%s", whereClause)
	t.Logf("")
	t.Logf("Generated Args:")
	argsJSON, _ := json.MarshalIndent(args, "", "  ")
	t.Logf("%s", string(argsJSON))
	t.Logf("======================================")

	// Expected: status = ? AND (major_code IN (?, ?) OR (semester_code = ? AND percent_stage_1 > ?))
	expectedArgs := []interface{}{"active", "CNTT", "KTPM", "HK1-2024", "70"}

	if len(args) != len(expectedArgs) {
		t.Errorf("Args length mismatch: expected %d, got %d", len(expectedArgs), len(args))
	}
}
