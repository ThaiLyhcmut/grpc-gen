package helper

import (
	"fmt"
	"strings"
	pbCommon "github.com/thaily/lms/proto/common"
)

// BuildFilterCondition builds SQL WHERE condition from FilterCondition (MySQL
// syntax with ?). Defensive: malformed conditions (missing/empty values for
// the operator) fall through to "1=1" rather than panicking on out-of-range
// access, so a client cannot crash the server by sending a partial filter.
func BuildFilterCondition(condition *pbCommon.FilterCondition, args *[]interface{}) string {
	field := condition.Field
	operator := condition.Operator
	values := condition.Values

	// Operators that need exactly one value
	switch operator {
	case pbCommon.FilterOperator_EQUAL,
		pbCommon.FilterOperator_NOT_EQUAL,
		pbCommon.FilterOperator_GREATER_THAN,
		pbCommon.FilterOperator_GREATER_THAN_EQUAL,
		pbCommon.FilterOperator_LESS_THAN,
		pbCommon.FilterOperator_LESS_THAN_EQUAL,
		pbCommon.FilterOperator_LIKE:
		if len(values) == 0 {
			return "1=1"
		}
	}

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
		if len(values) == 0 {
			return "1=1"
		}
		placeholders := []string{}
		for _, val := range values {
			*args = append(*args, val)
			placeholders = append(placeholders, "?")
		}
		return fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ", "))
	case pbCommon.FilterOperator_NOT_IN:
		if len(values) == 0 {
			return "1=1"
		}
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
		if len(values) < 2 {
			return "1=1"
		}
		*args = append(*args, values[0], values[1])
		return fmt.Sprintf("%s BETWEEN ? AND ?", field)
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

// FilterValidationError lists every field that appeared in the filter tree
// but was not present in the whitelist. Distinct, source-order preserved.
type FilterValidationError struct {
	InvalidFields []string
}

func (e *FilterValidationError) Error() string {
	return "filter fields not allowed: " + strings.Join(e.InvalidFields, ", ")
}

// BuildWhereClauseStrict is like BuildWhereClause but fails fast when any
// condition references a field outside whiteMap. It traverses the entire
// tree (including nested groups) and reports ALL invalid fields in one go,
// so the client can fix them in a single round-trip.
//
// On error, args is left untouched (no partial mutation).
func BuildWhereClauseStrict(filters []*pbCommon.FilterCriteria, args *[]interface{}, whiteMap map[string]bool) (string, error) {
	if len(filters) == 0 {
		return "", nil
	}

	invalid := collectInvalidFilterFields(filters, whiteMap)
	if len(invalid) > 0 {
		return "", &FilterValidationError{InvalidFields: invalid}
	}

	return BuildWhereClause(filters, args, whiteMap), nil
}

// collectInvalidFilterFields walks the filter tree and returns each distinct
// field name that is not in whiteMap, in the order first encountered.
func collectInvalidFilterFields(filters []*pbCommon.FilterCriteria, whiteMap map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(*pbCommon.FilterCriteria)
	walk = func(c *pbCommon.FilterCriteria) {
		if c == nil {
			return
		}
		if cond := c.GetCondition(); cond != nil {
			if _, ok := whiteMap[cond.Field]; !ok && !seen[cond.Field] {
				seen[cond.Field] = true
				out = append(out, cond.Field)
			}
			return
		}
		if grp := c.GetGroup(); grp != nil {
			for _, child := range grp.Filters {
				walk(child)
			}
		}
	}
	for _, f := range filters {
		walk(f)
	}
	return out
}
