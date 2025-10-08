package helper

import (
	"fmt"
	pbCommon "hehe/proto/common"
	"strings"
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
