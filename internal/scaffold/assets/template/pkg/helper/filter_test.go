package helper

import (
	"testing"
	pbCommon "thaily/proto/common"

	"github.com/stretchr/testify/assert"
)

func TestBuildFilterCondition(t *testing.T) {
	tests := []struct {
		name          string
		condition     *pbCommon.FilterCondition
		expectedSQL   string
		expectedArgs  []interface{}
	}{
		{
			name: "LIKE operator",
			condition: &pbCommon.FilterCondition{
				Field:    "title",
				Operator: pbCommon.FilterOperator_LIKE,
				Values:   []string{"10"},
			},
			expectedSQL:  "title LIKE ?",
			expectedArgs: []interface{}{"%10%"},
		},
		{
			name: "EQUAL operator",
			condition: &pbCommon.FilterCondition{
				Field:    "status",
				Operator: pbCommon.FilterOperator_EQUAL,
				Values:   []string{"active"},
			},
			expectedSQL:  "status = ?",
			expectedArgs: []interface{}{"active"},
		},
		{
			name: "IN operator",
			condition: &pbCommon.FilterCondition{
				Field:    "major_code",
				Operator: pbCommon.FilterOperator_IN,
				Values:   []string{"CNTT", "KTPM"},
			},
			expectedSQL:  "major_code IN (?, ?)",
			expectedArgs: []interface{}{"CNTT", "KTPM"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []interface{}{}
			sql := BuildFilterCondition(tt.condition, &args)

			assert.Equal(t, tt.expectedSQL, sql)
			assert.Equal(t, tt.expectedArgs, args)
		})
	}
}

func TestBuildNestedFilters(t *testing.T) {
	// Test case giống với JSON request của bạn
	// Filters: [
	//   {condition: title LIKE "10"},
	//   {group: {logic: OR, filters: [{condition: title LIKE "CNTT"}]}}
	// ]

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

	// Expected: (title LIKE '%10%') AND (title LIKE '%CNTT%')
	// Vì group chỉ có 1 filter nên OR không có tác dụng

	args := []interface{}{}
	conditions := []string{}

	// Test với logic CŨ (chỉ xử lý condition)
	for _, filter := range filters {
		if filter.GetCondition() != nil {
			condition := filter.GetCondition()
			sql := BuildFilterCondition(condition, &args)
			conditions = append(conditions, sql)
		}
	}

	t.Logf("OLD Logic - Generated SQL conditions: %v", conditions)
	t.Logf("OLD Logic - Generated Args: %v", args)

	// Với logic cũ, group filter bị bỏ qua
	assert.Equal(t, 1, len(conditions), "Logic cũ chỉ xử lý được condition, bỏ qua group")
	assert.Equal(t, []interface{}{"%10%"}, args)

	// Test với logic MỚI (sử dụng BuildFilterCriteria)
	args2 := []interface{}{}
	conditions2 := []string{}
	for _, filter := range filters {
		sql := BuildFilterCriteria(filter, &args2)
		if sql != "" && sql != "1=1" {
			conditions2 = append(conditions2, sql)
		}
	}

	t.Logf("NEW Logic - Generated SQL conditions: %v", conditions2)
	t.Logf("NEW Logic - Generated Args: %v", args2)

	// Với logic mới, cả 2 filters đều được xử lý
	assert.Equal(t, 2, len(conditions2), "Logic mới xử lý được cả condition và group")
	assert.Equal(t, []interface{}{"%10%", "%CNTT%"}, args2)
	assert.Equal(t, "title LIKE ?", conditions2[0])
	assert.Equal(t, "title LIKE ?", conditions2[1]) // Group chỉ có 1 filter nên không cần dấu ngoặc
}

func TestBuildNestedFiltersWithProperImplementation(t *testing.T) {
	// Test case phức tạp hơn với nhiều conditions trong group OR
	// Filters: [
	//   {condition: status = "active"},
	//   {group: {logic: OR, filters: [
	//     {condition: major_code = "CNTT"},
	//     {condition: major_code = "KTPM"}
	//   ]}}
	// ]
	// Expected SQL: status = ? AND (major_code = ? OR major_code = ?)
	// Expected Args: ["active", "CNTT", "KTPM"]

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
									Operator: pbCommon.FilterOperator_EQUAL,
									Values:   []string{"CNTT"},
								},
							},
						},
						{
							Criteria: &pbCommon.FilterCriteria_Condition{
								Condition: &pbCommon.FilterCondition{
									Field:    "major_code",
									Operator: pbCommon.FilterOperator_EQUAL,
									Values:   []string{"KTPM"},
								},
							},
						},
					},
				},
			},
		},
	}

	args := []interface{}{}
	conditions := []string{}
	for _, filter := range filters {
		sql := BuildFilterCriteria(filter, &args)
		if sql != "" && sql != "1=1" {
			conditions = append(conditions, sql)
		}
	}

	t.Logf("Generated SQL conditions: %v", conditions)
	t.Logf("Generated Args: %v", args)

	assert.Equal(t, 2, len(conditions))
	assert.Equal(t, "status = ?", conditions[0])
	assert.Equal(t, "(major_code = ? OR major_code = ?)", conditions[1])
	assert.Equal(t, []interface{}{"active", "CNTT", "KTPM"}, args)
}
