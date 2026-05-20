package handler

import (
	"context"
	"database/sql"
	"fmt"
	pb "github.com/thaily/lms/proto/class"
	commonpb "github.com/thaily/lms/proto/common"
	"github.com/thaily/lms/src/service/pkg/helper"
	"github.com/thaily/lms/src/service/pkg/logger"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// scanClass reads a single row into a *pb.Class.
// Shared by Create/Update inline-select and the List handler.
func scanClass(scanner interface{ Scan(...interface{}) error }) (*pb.Class, error) {
	var entity pb.Class
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var StatusStr string
	var SubjectNull sql.NullString
	var DescriptionNull sql.NullString
	var CoverUrlNull sql.NullString
	var MaxStudentsNull sql.NullInt32

	err := scanner.Scan(
		&entity.Id,
		&entity.TeacherId,
		&entity.Name,
		&SubjectNull,
		&DescriptionNull,
		&CoverUrlNull,
		&StatusStr,
		&MaxStudentsNull,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch StatusStr {
	case "class_status_unspecified":
		entity.Status = pb.ClassStatus_CLASS_STATUS_UNSPECIFIED
	case "draft":
		entity.Status = pb.ClassStatus_DRAFT
	case "active":
		entity.Status = pb.ClassStatus_ACTIVE
	case "archived":
		entity.Status = pb.ClassStatus_ARCHIVED
	default:
		entity.Status = pb.ClassStatus_CLASS_STATUS_UNSPECIFIED
	}

	if createdAt.Valid {
		entity.CreatedAt = timestamppb.New(createdAt.Time)
	}
	if updatedAt.Valid {
		entity.UpdatedAt = timestamppb.New(updatedAt.Time)
	}
	if createdBy.Valid {
		entity.CreatedBy = &createdBy.String
	}
	if updatedBy.Valid {
		entity.UpdatedBy = &updatedBy.String
	}
	if SubjectNull.Valid {
		val := SubjectNull.String
		entity.Subject = &val
	}
	if DescriptionNull.Valid {
		val := DescriptionNull.String
		entity.Description = &val
	}
	if CoverUrlNull.Valid {
		val := CoverUrlNull.String
		entity.CoverUrl = &val
	}
	if MaxStudentsNull.Valid {
		val := MaxStudentsNull.Int32
		entity.MaxStudents = &val
	}

	return &entity, nil
}

// buildClassWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in class_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildClassWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, ClassFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, ClassFilterableFields)
	return clause, args, err
}

// CreateClass creates a new Class record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateClass(ctx context.Context, req *pb.CreateClassRequest) (*pb.CreateClassResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	// === ID handling ===

	var id uint64
	id = req.GetId()
	var idArg interface{}
	if id != 0 {
		idArg = id
	}
	// idArg == nil → MySQL uses AUTO_INCREMENT and we read it back below.

	// Prepare fields. Use interface{} so nil propagates as SQL NULL rather than
	// silently becoming the zero value of the Go type (would lose presence info).
	// Optional string: Subject
	var Subject interface{}
	if req.Subject != nil {
		Subject = *req.Subject
	}
	// Optional string: Description
	var Description interface{}
	if req.Description != nil {
		Description = *req.Description
	}
	// Optional string: CoverUrl
	var CoverUrl interface{}
	if req.CoverUrl != nil {
		CoverUrl = *req.CoverUrl
	}
	// Optional int32: MaxStudents
	var MaxStudents interface{}
	if req.MaxStudents != nil {
		MaxStudents = *req.MaxStudents
	}

	// Convert Status enum to string
	StatusValue := pb.ClassStatus_CLASS_STATUS_UNSPECIFIED

	StatusValue = req.Status
	StatusStr := "class_status_unspecified"
	switch StatusValue {
	case pb.ClassStatus_CLASS_STATUS_UNSPECIFIED:
		StatusStr = "class_status_unspecified"
	case pb.ClassStatus_DRAFT:
		StatusStr = "draft"
	case pb.ClassStatus_ACTIVE:
		StatusStr = "active"
	case pb.ClassStatus_ARCHIVED:
		StatusStr = "archived"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO class (id, teacher_id, name, subject, description, cover_url, status, max_students, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.TeacherId,
		req.Name,
		Subject,
		Description,
		CoverUrl,
		StatusStr,
		MaxStudents,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "class already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create class: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetClass call).
	selectQuery := `
		SELECT id, teacher_id, name, subject, description, cover_url, status, max_students, created_at, updated_at, created_by, updated_by
		FROM class
		WHERE id = ?
	`
	entity, err := scanClass(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created class: %v", err)
	}

	return &pb.CreateClassResponse{
		Class: entity,
	}, nil
}

// UpdateClass applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateClass(ctx context.Context, req *pb.UpdateClassRequest) (*pb.UpdateClassResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildClassWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Name
	if req.Name != nil {
		updateFields = append(updateFields, "name = ?")
		args = append(args, *req.Name)

	}
	// Optional field: Subject
	if req.Subject != nil {
		updateFields = append(updateFields, "subject = ?")
		args = append(args, *req.Subject)

	}
	// Optional field: Description
	if req.Description != nil {
		updateFields = append(updateFields, "description = ?")
		args = append(args, *req.Description)

	}
	// Optional field: CoverUrl
	if req.CoverUrl != nil {
		updateFields = append(updateFields, "cover_url = ?")
		args = append(args, *req.CoverUrl)

	}
	// Optional field: Status
	if req.Status != nil {
		updateFields = append(updateFields, "status = ?")
		StatusStr := "class_status_unspecified"
		switch *req.Status {
		case pb.ClassStatus_CLASS_STATUS_UNSPECIFIED:
			StatusStr = "class_status_unspecified"
		case pb.ClassStatus_DRAFT:
			StatusStr = "draft"
		case pb.ClassStatus_ACTIVE:
			StatusStr = "active"
		case pb.ClassStatus_ARCHIVED:
			StatusStr = "archived"
		}
		args = append(args, StatusStr)

	}
	// Optional field: MaxStudents
	if req.MaxStudents != nil {
		updateFields = append(updateFields, "max_students = ?")
		args = append(args, *req.MaxStudents)

	}

	if len(updateFields) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no fields to update")
	}

	// updated_by / updated_at
	updateFields = append(updateFields, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	updateFields = append(updateFields, "updated_at = NOW()")

	// Append WHERE args after SET args
	args = append(args, whereArgs...)

	query := fmt.Sprintf(`UPDATE class SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update class: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, teacher_id, name, subject, description, cover_url, status, max_students, created_at, updated_at, created_by, updated_by FROM class %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated classs: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Class{}
	for rows.Next() {
		entity, err := scanClass(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan class: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating classs: %v", err)
	}

	return &pb.UpdateClassResponse{
		Class:         entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteClass deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteClass(ctx context.Context, req *pb.DeleteClassRequest) (*pb.DeleteClassResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildClassWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM class %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete class: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteClassResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListClass lists Classs with pagination and filtering
func (h *Handler) ListClass(ctx context.Context, req *pb.ListClassRequest) (*pb.ListClassResponse, error) {
	defer logger.TraceFunction(ctx)()

	page := int32(1)
	pageSize := int32(10)
	sortBy := "created_at"
	descending := true
	if req.Search != nil && req.Search.Pagination != nil {
		if req.Search.Pagination.Page > 0 {
			page = req.Search.Pagination.Page
		}
		if req.Search.Pagination.PageSize > 0 {
			pageSize = req.Search.Pagination.PageSize
		}
		if req.Search.Pagination.SortBy != "" {
			sortBy = req.Search.Pagination.SortBy
		}
		descending = req.Search.Pagination.Descending
	}

	offset := (page - 1) * pageSize

	whereClause := ""
	args := []interface{}{}
	if req.Search != nil && len(req.Search.Filters) > 0 {
		var err error
		whereClause, args, err = buildClassWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM class %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count classs: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, teacher_id, name, subject, description, cover_url, status, max_students, created_at, updated_at, created_by, updated_by
		FROM class
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list classs: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Class{}
	for rows.Next() {
		entity, err := scanClass(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan class: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating classs: %v", err)
	}

	return &pb.ListClassResponse{
		Class:    entities,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
