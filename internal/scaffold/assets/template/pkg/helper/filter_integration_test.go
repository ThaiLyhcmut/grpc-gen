package helper

import (
	"fmt"
	"strings"
	"testing"
	pbCommon "thaily/proto/common"

	"github.com/stretchr/testify/assert"
)

// TestFilterIntegrationWithWhitelist tests the complete filter flow with whitelist validation
// This simulates how the handler (e.g., topic.go) would use the filter functions
func TestFilterIntegrationWithWhitelist(t *testing.T) {
	// Whitelist map (same as in topic.go handler)
	whiteMap := map[string]bool{
		"id":              true,
		"title":           true,
		"major_code":      true,
		"semester_code":   true,
		"status":          true,
		"percent_stage_1": true,
		"percent_stage_2": true,
	}

	// Test case: Your exact JSON request
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

	// Build WHERE clause (same pattern as in topic.go handler)
	whereClause := ""
	args := []interface{}{}

	if len(filters) > 0 {
		whereConditions := []string{}
		for _, filter := range filters {
			condition := BuildFilterCriteriaWithWhitelist(filter, &args, whiteMap)
			if condition != "" && condition != "1=1" {
				whereConditions = append(whereConditions, condition)
			}
		}
		if len(whereConditions) > 0 {
			whereClause = "WHERE " + strings.Join(whereConditions, " AND ")
		}
	}

	// Expected results
	expectedWhereClause := "WHERE title LIKE ? AND title LIKE ?"
	expectedArgs := []interface{}{"%10%", "%CNTT%"}

	assert.Equal(t, expectedWhereClause, whereClause, "WHERE clause should match")
	assert.Equal(t, expectedArgs, args, "Args should match")

	// Print full SQL
	fullSQL := fmt.Sprintf("SELECT * FROM Topic %s ORDER BY created_at ASC LIMIT 100 OFFSET 0", whereClause)
	t.Logf("Generated SQL: %s", fullSQL)
	t.Logf("With Args: %v", args)
}

// TestFilterIntegrationWithInvalidField tests that invalid fields are filtered out
func TestFilterIntegrationWithInvalidField(t *testing.T) {
	whiteMap := map[string]bool{
		"title":      true,
		"major_code": true,
	}

	// Include an invalid field "hacker_field"
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
			Criteria: &pbCommon.FilterCriteria_Condition{
				Condition: &pbCommon.FilterCondition{
					Field:    "hacker_field", // Invalid field!
					Operator: pbCommon.FilterOperator_EQUAL,
					Values:   []string{"malicious"},
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
									Operator: pbCommon.FilterOperator_EQUAL,
									Values:   []string{"CNTT"},
								},
							},
						},
						{
							Criteria: &pbCommon.FilterCriteria_Condition{
								Condition: &pbCommon.FilterCondition{
									Field:    "another_invalid", // Invalid field!
									Operator: pbCommon.FilterOperator_EQUAL,
									Values:   []string{"bad"},
								},
							},
						},
					},
				},
			},
		},
	}

	whereClause := ""
	args := []interface{}{}

	if len(filters) > 0 {
		whereConditions := []string{}
		for _, filter := range filters {
			condition := BuildFilterCriteriaWithWhitelist(filter, &args, whiteMap)
			if condition != "" && condition != "1=1" {
				whereConditions = append(whereConditions, condition)
			}
		}
		if len(whereConditions) > 0 {
			whereClause = "WHERE " + strings.Join(whereConditions, " AND ")
		}
	}

	// Should only include valid fields
	expectedWhereClause := "WHERE title LIKE ? AND major_code = ?"
	expectedArgs := []interface{}{"%10%", "CNTT"}

	assert.Equal(t, expectedWhereClause, whereClause, "Invalid fields should be filtered out")
	assert.Equal(t, expectedArgs, args, "Args should only include valid fields")

	t.Logf("Successfully filtered out invalid fields!")
	t.Logf("Generated SQL: SELECT * FROM Topic %s", whereClause)
	t.Logf("With Args: %v", args)
}

// TestFilterIntegrationComplexNested tests complex nested structure with whitelist
func TestFilterIntegrationComplexNested(t *testing.T) {
	whiteMap := map[string]bool{
		"status":          true,
		"major_code":      true,
		"semester_code":   true,
		"percent_stage_1": true,
	}

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

	whereClause := ""
	args := []interface{}{}

	if len(filters) > 0 {
		whereConditions := []string{}
		for _, filter := range filters {
			condition := BuildFilterCriteriaWithWhitelist(filter, &args, whiteMap)
			if condition != "" && condition != "1=1" {
				whereConditions = append(whereConditions, condition)
			}
		}
		if len(whereConditions) > 0 {
			whereClause = "WHERE " + strings.Join(whereConditions, " AND ")
		}
	}

	expectedWhereClause := "WHERE status = ? AND (major_code IN (?, ?) OR (semester_code = ? AND percent_stage_1 > ?))"
	expectedArgs := []interface{}{"active", "CNTT", "KTPM", "HK1-2024", "70"}

	assert.Equal(t, expectedWhereClause, whereClause)
	assert.Equal(t, expectedArgs, args)

	t.Logf("Complex nested filter test passed!")
	t.Logf("Generated SQL: SELECT * FROM Topic %s", whereClause)
	t.Logf("With Args: %v", args)
}
