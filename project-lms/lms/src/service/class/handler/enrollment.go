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

// scanEnrollment reads a single row into a *pb.Enrollment.
// Shared by Create/Update inline-select and the List handler.
func scanEnrollment(scanner interface{ Scan(...interface{}) error }) (*pb.Enrollment, error) {
	var entity pb.Enrollment
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var StatusStr string
	var JoinedAtTime sql.NullTime
	var RemovedAtTime sql.NullTime
	var InviteKeyIdNull sql.NullInt64
	var RemovedByUserIdNull sql.NullInt64

	err := scanner.Scan(
		&entity.Id,
		&entity.ClassId,
		&entity.StudentId,
		&InviteKeyIdNull,
		&JoinedAtTime,
		&StatusStr,
		&RemovedAtTime,
		&RemovedByUserIdNull,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch StatusStr {
	case "enrollment_status_unspecified":
		entity.Status = pb.EnrollmentStatus_ENROLLMENT_STATUS_UNSPECIFIED
	case "enroll_active":
		entity.Status = pb.EnrollmentStatus_ENROLL_ACTIVE
	case "enroll_removed":
		entity.Status = pb.EnrollmentStatus_ENROLL_REMOVED
	default:
		entity.Status = pb.EnrollmentStatus_ENROLLMENT_STATUS_UNSPECIFIED
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
	if JoinedAtTime.Valid {
		entity.JoinedAt = timestamppb.New(JoinedAtTime.Time)
	}
	if RemovedAtTime.Valid {
		entity.RemovedAt = timestamppb.New(RemovedAtTime.Time)
	}
	if InviteKeyIdNull.Valid {
		val := uint64(InviteKeyIdNull.Int64)
		entity.InviteKeyId = &val
	}
	if RemovedByUserIdNull.Valid {
		val := uint64(RemovedByUserIdNull.Int64)
		entity.RemovedByUserId = &val
	}

	return &entity, nil
}

// buildEnrollmentWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in enrollment_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildEnrollmentWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, EnrollmentFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, EnrollmentFilterableFields)
	return clause, args, err
}

// CreateEnrollment creates a new Enrollment record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateEnrollment(ctx context.Context, req *pb.CreateEnrollmentRequest) (*pb.CreateEnrollmentResponse, error) {
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
	// Optional uint64: InviteKeyId
	var InviteKeyId interface{}
	if req.InviteKeyId != nil {
		InviteKeyId = *req.InviteKeyId
	}
	// Optional timestamp: RemovedAt
	var RemovedAt interface{}
	if req.RemovedAt != nil {
		RemovedAt = req.RemovedAt.AsTime()
	}
	// Optional uint64: RemovedByUserId
	var RemovedByUserId interface{}
	if req.RemovedByUserId != nil {
		RemovedByUserId = *req.RemovedByUserId
	}

	// Convert Status enum to string
	StatusValue := pb.EnrollmentStatus_ENROLLMENT_STATUS_UNSPECIFIED

	StatusValue = req.Status
	StatusStr := "enrollment_status_unspecified"
	switch StatusValue {
	case pb.EnrollmentStatus_ENROLLMENT_STATUS_UNSPECIFIED:
		StatusStr = "enrollment_status_unspecified"
	case pb.EnrollmentStatus_ENROLL_ACTIVE:
		StatusStr = "enroll_active"
	case pb.EnrollmentStatus_ENROLL_REMOVED:
		StatusStr = "enroll_removed"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO enrollment (id, class_id, student_id, invite_key_id, joined_at, status, removed_at, removed_by_user_id, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.ClassId,
		req.StudentId,
		InviteKeyId,
		req.JoinedAt.AsTime(),
		StatusStr,
		RemovedAt,
		RemovedByUserId,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "enrollment already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create enrollment: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetEnrollment call).
	selectQuery := `
		SELECT id, class_id, student_id, invite_key_id, joined_at, status, removed_at, removed_by_user_id, created_at, updated_at, created_by, updated_by
		FROM enrollment
		WHERE id = ?
	`
	entity, err := scanEnrollment(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created enrollment: %v", err)
	}

	return &pb.CreateEnrollmentResponse{
		Enrollment: entity,
	}, nil
}

// UpdateEnrollment applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateEnrollment(ctx context.Context, req *pb.UpdateEnrollmentRequest) (*pb.UpdateEnrollmentResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildEnrollmentWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Status
	if req.Status != nil {
		updateFields = append(updateFields, "status = ?")
		StatusStr := "enrollment_status_unspecified"
		switch *req.Status {
		case pb.EnrollmentStatus_ENROLLMENT_STATUS_UNSPECIFIED:
			StatusStr = "enrollment_status_unspecified"
		case pb.EnrollmentStatus_ENROLL_ACTIVE:
			StatusStr = "enroll_active"
		case pb.EnrollmentStatus_ENROLL_REMOVED:
			StatusStr = "enroll_removed"
		}
		args = append(args, StatusStr)

	}
	// Optional field: RemovedAt
	if req.RemovedAt != nil {
		updateFields = append(updateFields, "removed_at = ?")
		args = append(args, req.RemovedAt.AsTime())

	}
	// Optional field: RemovedByUserId
	if req.RemovedByUserId != nil {
		updateFields = append(updateFields, "removed_by_user_id = ?")
		args = append(args, *req.RemovedByUserId)

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

	query := fmt.Sprintf(`UPDATE enrollment SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update enrollment: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, class_id, student_id, invite_key_id, joined_at, status, removed_at, removed_by_user_id, created_at, updated_at, created_by, updated_by FROM enrollment %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated enrollments: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Enrollment{}
	for rows.Next() {
		entity, err := scanEnrollment(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan enrollment: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating enrollments: %v", err)
	}

	return &pb.UpdateEnrollmentResponse{
		Enrollment:    entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteEnrollment deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteEnrollment(ctx context.Context, req *pb.DeleteEnrollmentRequest) (*pb.DeleteEnrollmentResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildEnrollmentWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM enrollment %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete enrollment: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteEnrollmentResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListEnrollment lists Enrollments with pagination and filtering
func (h *Handler) ListEnrollment(ctx context.Context, req *pb.ListEnrollmentRequest) (*pb.ListEnrollmentResponse, error) {
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
		whereClause, args, err = buildEnrollmentWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM enrollment %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count enrollments: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, class_id, student_id, invite_key_id, joined_at, status, removed_at, removed_by_user_id, created_at, updated_at, created_by, updated_by
		FROM enrollment
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list enrollments: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Enrollment{}
	for rows.Next() {
		entity, err := scanEnrollment(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan enrollment: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating enrollments: %v", err)
	}

	return &pb.ListEnrollmentResponse{
		Enrollment: entities,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
	}, nil
}
