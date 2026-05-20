package handler

import (
	"context"
	"database/sql"
	"fmt"
	commonpb "github.com/thaily/lms/proto/common"
	pb "github.com/thaily/lms/proto/user"
	"github.com/thaily/lms/src/service/pkg/helper"
	"github.com/thaily/lms/src/service/pkg/logger"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// scanStudentProfile reads a single row into a *pb.StudentProfile.
// Shared by Create/Update inline-select and the List handler.
func scanStudentProfile(scanner interface{ Scan(...interface{}) error }) (*pb.StudentProfile, error) {
	var entity pb.StudentProfile
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var GradeNull sql.NullString
	var SchoolNameNull sql.NullString
	var ParentNameNull sql.NullString
	var ParentPhoneNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.UserId,
		&GradeNull,
		&SchoolNameNull,
		&ParentNameNull,
		&ParentPhoneNull,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
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
	if GradeNull.Valid {
		val := GradeNull.String
		entity.Grade = &val
	}
	if SchoolNameNull.Valid {
		val := SchoolNameNull.String
		entity.SchoolName = &val
	}
	if ParentNameNull.Valid {
		val := ParentNameNull.String
		entity.ParentName = &val
	}
	if ParentPhoneNull.Valid {
		val := ParentPhoneNull.String
		entity.ParentPhone = &val
	}

	return &entity, nil
}

// buildStudentProfileWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in studentprofile_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildStudentProfileWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, StudentProfileFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, StudentProfileFilterableFields)
	return clause, args, err
}

// CreateStudentProfile creates a new StudentProfile record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateStudentProfile(ctx context.Context, req *pb.CreateStudentProfileRequest) (*pb.CreateStudentProfileResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)

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
	// Optional string: Grade
	var Grade interface{}
	if req.Grade != nil {
		Grade = *req.Grade
	}
	// Optional string: SchoolName
	var SchoolName interface{}
	if req.SchoolName != nil {
		SchoolName = *req.SchoolName
	}
	// Optional string: ParentName
	var ParentName interface{}
	if req.ParentName != nil {
		ParentName = *req.ParentName
	}
	// Optional string: ParentPhone
	var ParentPhone interface{}
	if req.ParentPhone != nil {
		ParentPhone = *req.ParentPhone
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO studentprofile (id, user_id, grade, school_name, parent_name, parent_phone, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.UserId,
		Grade,
		SchoolName,
		ParentName,
		ParentPhone,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "studentprofile already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create studentprofile: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetStudentProfile call).
	selectQuery := `
		SELECT id, user_id, grade, school_name, parent_name, parent_phone, created_at, updated_at, created_by, updated_by
		FROM studentprofile
		WHERE id = ?
	`
	entity, err := scanStudentProfile(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created studentprofile: %v", err)
	}

	return &pb.CreateStudentProfileResponse{
		StudentProfile: entity,
	}, nil
}

// UpdateStudentProfile applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateStudentProfile(ctx context.Context, req *pb.UpdateStudentProfileRequest) (*pb.UpdateStudentProfileResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildStudentProfileWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Grade
	if req.Grade != nil {
		updateFields = append(updateFields, "grade = ?")
		args = append(args, *req.Grade)

	}
	// Optional field: SchoolName
	if req.SchoolName != nil {
		updateFields = append(updateFields, "school_name = ?")
		args = append(args, *req.SchoolName)

	}
	// Optional field: ParentName
	if req.ParentName != nil {
		updateFields = append(updateFields, "parent_name = ?")
		args = append(args, *req.ParentName)

	}
	// Optional field: ParentPhone
	if req.ParentPhone != nil {
		updateFields = append(updateFields, "parent_phone = ?")
		args = append(args, *req.ParentPhone)

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

	query := fmt.Sprintf(`UPDATE studentprofile SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update studentprofile: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, user_id, grade, school_name, parent_name, parent_phone, created_at, updated_at, created_by, updated_by FROM studentprofile %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated studentprofiles: %v", err)
	}
	defer rows.Close()

	entities := []*pb.StudentProfile{}
	for rows.Next() {
		entity, err := scanStudentProfile(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan studentprofile: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating studentprofiles: %v", err)
	}

	return &pb.UpdateStudentProfileResponse{
		StudentProfile: entities,
		AffectedCount:  int32(affected),
	}, nil
}

// DeleteStudentProfile deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteStudentProfile(ctx context.Context, req *pb.DeleteStudentProfileRequest) (*pb.DeleteStudentProfileResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildStudentProfileWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM studentprofile %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete studentprofile: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteStudentProfileResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListStudentProfile lists StudentProfiles with pagination and filtering
func (h *Handler) ListStudentProfile(ctx context.Context, req *pb.ListStudentProfileRequest) (*pb.ListStudentProfileResponse, error) {
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
		whereClause, args, err = buildStudentProfileWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM studentprofile %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count studentprofiles: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, user_id, grade, school_name, parent_name, parent_phone, created_at, updated_at, created_by, updated_by
		FROM studentprofile
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list studentprofiles: %v", err)
	}
	defer rows.Close()

	entities := []*pb.StudentProfile{}
	for rows.Next() {
		entity, err := scanStudentProfile(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan studentprofile: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating studentprofiles: %v", err)
	}

	return &pb.ListStudentProfileResponse{
		StudentProfile: entities,
		Total:          total,
		Page:           page,
		PageSize:       pageSize,
	}, nil
}
