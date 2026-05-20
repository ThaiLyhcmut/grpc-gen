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

// scanTeacherProfile reads a single row into a *pb.TeacherProfile.
// Shared by Create/Update inline-select and the List handler.
func scanTeacherProfile(scanner interface{ Scan(...interface{}) error }) (*pb.TeacherProfile, error) {
	var entity pb.TeacherProfile
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var BioNull sql.NullString
	var SubjectsNull sql.NullString
	var YearsExpNull sql.NullInt32
	var FacebookUrlNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.UserId,
		&BioNull,
		&SubjectsNull,
		&YearsExpNull,
		&FacebookUrlNull,
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
	if BioNull.Valid {
		val := BioNull.String
		entity.Bio = &val
	}
	if SubjectsNull.Valid {
		val := SubjectsNull.String
		entity.Subjects = &val
	}
	if YearsExpNull.Valid {
		val := YearsExpNull.Int32
		entity.YearsExp = &val
	}
	if FacebookUrlNull.Valid {
		val := FacebookUrlNull.String
		entity.FacebookUrl = &val
	}

	return &entity, nil
}

// buildTeacherProfileWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in teacherprofile_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildTeacherProfileWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, TeacherProfileFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, TeacherProfileFilterableFields)
	return clause, args, err
}

// CreateTeacherProfile creates a new TeacherProfile record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateTeacherProfile(ctx context.Context, req *pb.CreateTeacherProfileRequest) (*pb.CreateTeacherProfileResponse, error) {
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
	// Optional string: Bio
	var Bio interface{}
	if req.Bio != nil {
		Bio = *req.Bio
	}
	// Optional string: Subjects
	var Subjects interface{}
	if req.Subjects != nil {
		Subjects = *req.Subjects
	}
	// Optional int32: YearsExp
	var YearsExp interface{}
	if req.YearsExp != nil {
		YearsExp = *req.YearsExp
	}
	// Optional string: FacebookUrl
	var FacebookUrl interface{}
	if req.FacebookUrl != nil {
		FacebookUrl = *req.FacebookUrl
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO teacherprofile (id, user_id, bio, subjects, years_exp, facebook_url, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.UserId,
		Bio,
		Subjects,
		YearsExp,
		FacebookUrl,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "teacherprofile already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create teacherprofile: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetTeacherProfile call).
	selectQuery := `
		SELECT id, user_id, bio, subjects, years_exp, facebook_url, created_at, updated_at, created_by, updated_by
		FROM teacherprofile
		WHERE id = ?
	`
	entity, err := scanTeacherProfile(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created teacherprofile: %v", err)
	}

	return &pb.CreateTeacherProfileResponse{
		TeacherProfile: entity,
	}, nil
}

// UpdateTeacherProfile applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateTeacherProfile(ctx context.Context, req *pb.UpdateTeacherProfileRequest) (*pb.UpdateTeacherProfileResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildTeacherProfileWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Bio
	if req.Bio != nil {
		updateFields = append(updateFields, "bio = ?")
		args = append(args, *req.Bio)

	}
	// Optional field: Subjects
	if req.Subjects != nil {
		updateFields = append(updateFields, "subjects = ?")
		args = append(args, *req.Subjects)

	}
	// Optional field: YearsExp
	if req.YearsExp != nil {
		updateFields = append(updateFields, "years_exp = ?")
		args = append(args, *req.YearsExp)

	}
	// Optional field: FacebookUrl
	if req.FacebookUrl != nil {
		updateFields = append(updateFields, "facebook_url = ?")
		args = append(args, *req.FacebookUrl)

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

	query := fmt.Sprintf(`UPDATE teacherprofile SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update teacherprofile: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, user_id, bio, subjects, years_exp, facebook_url, created_at, updated_at, created_by, updated_by FROM teacherprofile %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated teacherprofiles: %v", err)
	}
	defer rows.Close()

	entities := []*pb.TeacherProfile{}
	for rows.Next() {
		entity, err := scanTeacherProfile(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan teacherprofile: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating teacherprofiles: %v", err)
	}

	return &pb.UpdateTeacherProfileResponse{
		TeacherProfile: entities,
		AffectedCount:  int32(affected),
	}, nil
}

// DeleteTeacherProfile deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteTeacherProfile(ctx context.Context, req *pb.DeleteTeacherProfileRequest) (*pb.DeleteTeacherProfileResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildTeacherProfileWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM teacherprofile %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete teacherprofile: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteTeacherProfileResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListTeacherProfile lists TeacherProfiles with pagination and filtering
func (h *Handler) ListTeacherProfile(ctx context.Context, req *pb.ListTeacherProfileRequest) (*pb.ListTeacherProfileResponse, error) {
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
		whereClause, args, err = buildTeacherProfileWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM teacherprofile %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count teacherprofiles: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, user_id, bio, subjects, years_exp, facebook_url, created_at, updated_at, created_by, updated_by
		FROM teacherprofile
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list teacherprofiles: %v", err)
	}
	defer rows.Close()

	entities := []*pb.TeacherProfile{}
	for rows.Next() {
		entity, err := scanTeacherProfile(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan teacherprofile: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating teacherprofiles: %v", err)
	}

	return &pb.ListTeacherProfileResponse{
		TeacherProfile: entities,
		Total:          total,
		Page:           page,
		PageSize:       pageSize,
	}, nil
}
