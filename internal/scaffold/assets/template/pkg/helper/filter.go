package helper

import (
	"fmt"
	"strings"
	pbCommon "thaily/proto/common"
)

// BuildFilterCondition builds SQL WHERE condition from FilterCondition (MySQL syntax with ?)
func BuildFilterCondition(condition *pbCommon.FilterCondition, args *[]interface{}) string {
	field := condition.Field
	operator := condition.Operator
	values := condition.Values

	switch operator {
	case pbCommon.FilterOperator_EQUAL:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s = ?", field)
	case pbCommon.FilterOperator_NOT_EQUAL:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s != ?", field)
	case pbCommon.FilterOperator_GREATER_THAN:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s > ?", field)
	case pbCommon.FilterOperator_GREATER_THAN_EQUAL:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s >= ?", field)
	case pbCommon.FilterOperator_LESS_THAN:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s < ?", field)
	case pbCommon.FilterOperator_LESS_THAN_EQUAL:
		*args = append(*args, values[0])
		return fmt.Sprintf("%s <= ?", field)
	case pbCommon.FilterOperator_LIKE:
		*args = append(*args, "%"+values[0]+"%")
		return fmt.Sprintf("%s LIKE ?", field)
	case pbCommon.FilterOperator_IN:
		placeholders := []string{}
		for _, val := range values {
			*args = append(*args, val)
			placeholders = append(placeholders, "?")
		}
		return fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ", "))
	case pbCommon.FilterOperator_NOT_IN:
		placeholders := []string{}
		for _, val := range values {
			*args = append(*args, val)
			placeholders = append(placeholders, "?")
		}
		return fmt.Sprintf("%s NOT IN (%s)", field, strings.Join(placeholders, ", "))
	case pbCommon.FilterOperator_IS_NULL:
		return fmt.Sprintf("%s IS NULL", field)
	case pbCommon.FilterOperator_IS_NOT_NULL:
		return fmt.Sprintf("%s IS NOT NULL", field)
	case pbCommon.FilterOperator_BETWEEN:
		if len(values) >= 2 {
			*args = append(*args, values[0], values[1])
			return fmt.Sprintf("%s BETWEEN ? AND ?", field)
		}
	}

	return "1=1" // fallback
}

// BuildFilterGroup builds SQL WHERE condition from FilterGroup with nested support
func BuildFilterGroup(group *pbCommon.FilterGroup, args *[]interface{}) string {
	if group == nil || len(group.Filters) == 0 {
		return "1=1"
	}

	conditions := []string{}
	for _, filter := range group.Filters {
		condition := BuildFilterCriteria(filter, args)
		if condition != "" && condition != "1=1" {
			conditions = append(conditions, condition)
		}
	}

	if len(conditions) == 0 {
		return "1=1"
	}

	// Join with logic operator (AND/OR)
	logicOp := "AND"
	if group.Logic == pbCommon.LogicalCondition_OR {
		logicOp = "OR"
	}

	// If only one condition, no need for parentheses
	if len(conditions) == 1 {
		return conditions[0]
	}

	// Multiple conditions - wrap in parentheses and join with logic operator
	return "(" + strings.Join(conditions, " "+logicOp+" ") + ")"
}

// BuildFilterCriteria builds SQL WHERE condition from FilterCriteria (handles both condition and group)
func BuildFilterCriteria(criteria *pbCommon.FilterCriteria, args *[]interface{}) string {
	if criteria == nil {
		return "1=1"
	}

	if condition := criteria.GetCondition(); condition != nil {
		return BuildFilterCondition(condition, args)
	}

	if group := criteria.GetGroup(); group != nil {
		return BuildFilterGroup(group, args)
	}

	return "1=1"
}

// BuildFilterCriteriaWithWhitelist builds SQL WHERE condition with field whitelist validation
func BuildFilterCriteriaWithWhitelist(criteria *pbCommon.FilterCriteria, args *[]interface{}, whiteMap map[string]bool) string {
	if criteria == nil {
		return "1=1"
	}

	if condition := criteria.GetCondition(); condition != nil {
		// Validate field against whitelist
		if whiteMap != nil {
			if _, ok := whiteMap[condition.Field]; !ok {
				return "1=1" // Skip invalid field
			}
		}
		return BuildFilterCondition(condition, args)
	}

	if group := criteria.GetGroup(); group != nil {
		return BuildFilterGroupWithWhitelist(group, args, whiteMap)
	}

	return "1=1"
}

// BuildFilterGroupWithWhitelist builds SQL WHERE condition from FilterGroup with field validation
func BuildFilterGroupWithWhitelist(group *pbCommon.FilterGroup, args *[]interface{}, whiteMap map[string]bool) string {
	if group == nil || len(group.Filters) == 0 {
		return "1=1"
	}

	conditions := []string{}
	for _, filter := range group.Filters {
		condition := BuildFilterCriteriaWithWhitelist(filter, args, whiteMap)
		if condition != "" && condition != "1=1" {
			conditions = append(conditions, condition)
		}
	}

	if len(conditions) == 0 {
		return "1=1"
	}

	// Join with logic operator (AND/OR)
	logicOp := "AND"
	if group.Logic == pbCommon.LogicalCondition_OR {
		logicOp = "OR"
	}

	// If only one condition, no need for parentheses
	if len(conditions) == 1 {
		return conditions[0]
	}

	// Multiple conditions - wrap in parentheses and join with logic operator
	return "(" + strings.Join(conditions, " "+logicOp+" ") + ")"
}

// BuildWhereClause is a high-level helper that builds complete WHERE clause from filters
// This is the recommended function to use in handlers for consistency
// It handles both simple conditions and nested groups with field validation
func BuildWhereClause(filters []*pbCommon.FilterCriteria, args *[]interface{}, whiteMap map[string]bool) string {
	if len(filters) == 0 {
		return ""
	}

	whereConditions := []string{}
	for _, filter := range filters {
		condition := BuildFilterCriteriaWithWhitelist(filter, args, whiteMap)
		if condition != "" && condition != "1=1" {
			whereConditions = append(whereConditions, condition)
		}
	}

	if len(whereConditions) == 0 {
		return ""
	}

	return "WHERE " + strings.Join(whereConditions, " AND ")
}
