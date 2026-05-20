package handler

import (
	"context"
	"database/sql"
	"fmt"
	commonpb "github.com/thaily/lms/proto/common"
	pb "github.com/thaily/lms/proto/lesson"
	"github.com/thaily/lms/src/service/pkg/helper"
	"github.com/thaily/lms/src/service/pkg/logger"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// scanLessonAttendance reads a single row into a *pb.LessonAttendance.
// Shared by Create/Update inline-select and the List handler.
func scanLessonAttendance(scanner interface{ Scan(...interface{}) error }) (*pb.LessonAttendance, error) {
	var entity pb.LessonAttendance
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var StatusStr string
	var MarkedAtTime sql.NullTime
	var NoteNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.LessonId,
		&entity.StudentId,
		&StatusStr,
		&NoteNull,
		&entity.MarkedByTeacherId,
		&MarkedAtTime,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch StatusStr {
	case "attendance_status_unspecified":
		entity.Status = pb.AttendanceStatus_ATTENDANCE_STATUS_UNSPECIFIED
	case "present":
		entity.Status = pb.AttendanceStatus_PRESENT
	case "absent":
		entity.Status = pb.AttendanceStatus_ABSENT
	case "late":
		entity.Status = pb.AttendanceStatus_LATE
	case "excused":
		entity.Status = pb.AttendanceStatus_EXCUSED
	default:
		entity.Status = pb.AttendanceStatus_ATTENDANCE_STATUS_UNSPECIFIED
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
	if MarkedAtTime.Valid {
		entity.MarkedAt = timestamppb.New(MarkedAtTime.Time)
	}
	if NoteNull.Valid {
		val := NoteNull.String
		entity.Note = &val
	}

	return &entity, nil
}

// buildLessonAttendanceWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in lessonattendance_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildLessonAttendanceWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, LessonAttendanceFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, LessonAttendanceFilterableFields)
	return clause, args, err
}

// CreateLessonAttendance creates a new LessonAttendance record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateLessonAttendance(ctx context.Context, req *pb.CreateLessonAttendanceRequest) (*pb.CreateLessonAttendanceResponse, error) {
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
	// Optional string: Note
	var Note interface{}
	if req.Note != nil {
		Note = *req.Note
	}

	// Convert Status enum to string
	StatusValue := pb.AttendanceStatus_ATTENDANCE_STATUS_UNSPECIFIED

	StatusValue = req.Status
	StatusStr := "attendance_status_unspecified"
	switch StatusValue {
	case pb.AttendanceStatus_ATTENDANCE_STATUS_UNSPECIFIED:
		StatusStr = "attendance_status_unspecified"
	case pb.AttendanceStatus_PRESENT:
		StatusStr = "present"
	case pb.AttendanceStatus_ABSENT:
		StatusStr = "absent"
	case pb.AttendanceStatus_LATE:
		StatusStr = "late"
	case pb.AttendanceStatus_EXCUSED:
		StatusStr = "excused"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO lessonattendance (id, lesson_id, student_id, status, note, marked_by_teacher_id, marked_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.LessonId,
		req.StudentId,
		StatusStr,
		Note,
		req.MarkedByTeacherId,
		req.MarkedAt.AsTime(),
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "lessonattendance already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create lessonattendance: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetLessonAttendance call).
	selectQuery := `
		SELECT id, lesson_id, student_id, status, note, marked_by_teacher_id, marked_at, created_at, updated_at, created_by, updated_by
		FROM lessonattendance
		WHERE id = ?
	`
	entity, err := scanLessonAttendance(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created lessonattendance: %v", err)
	}

	return &pb.CreateLessonAttendanceResponse{
		LessonAttendance: entity,
	}, nil
}

// UpdateLessonAttendance applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateLessonAttendance(ctx context.Context, req *pb.UpdateLessonAttendanceRequest) (*pb.UpdateLessonAttendanceResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildLessonAttendanceWhere(req.GetFilters())
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
		StatusStr := "attendance_status_unspecified"
		switch *req.Status {
		case pb.AttendanceStatus_ATTENDANCE_STATUS_UNSPECIFIED:
			StatusStr = "attendance_status_unspecified"
		case pb.AttendanceStatus_PRESENT:
			StatusStr = "present"
		case pb.AttendanceStatus_ABSENT:
			StatusStr = "absent"
		case pb.AttendanceStatus_LATE:
			StatusStr = "late"
		case pb.AttendanceStatus_EXCUSED:
			StatusStr = "excused"
		}
		args = append(args, StatusStr)

	}
	// Optional field: Note
	if req.Note != nil {
		updateFields = append(updateFields, "note = ?")
		args = append(args, *req.Note)

	}
	// Optional field: MarkedAt
	if req.MarkedAt != nil {
		updateFields = append(updateFields, "marked_at = ?")
		args = append(args, req.MarkedAt.AsTime())

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

	query := fmt.Sprintf(`UPDATE lessonattendance SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update lessonattendance: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, lesson_id, student_id, status, note, marked_by_teacher_id, marked_at, created_at, updated_at, created_by, updated_by FROM lessonattendance %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated lessonattendances: %v", err)
	}
	defer rows.Close()

	entities := []*pb.LessonAttendance{}
	for rows.Next() {
		entity, err := scanLessonAttendance(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan lessonattendance: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating lessonattendances: %v", err)
	}

	return &pb.UpdateLessonAttendanceResponse{
		LessonAttendance: entities,
		AffectedCount:    int32(affected),
	}, nil
}

// DeleteLessonAttendance deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteLessonAttendance(ctx context.Context, req *pb.DeleteLessonAttendanceRequest) (*pb.DeleteLessonAttendanceResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildLessonAttendanceWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM lessonattendance %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete lessonattendance: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteLessonAttendanceResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListLessonAttendance lists LessonAttendances with pagination and filtering
func (h *Handler) ListLessonAttendance(ctx context.Context, req *pb.ListLessonAttendanceRequest) (*pb.ListLessonAttendanceResponse, error) {
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
		whereClause, args, err = buildLessonAttendanceWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM lessonattendance %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count lessonattendances: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, lesson_id, student_id, status, note, marked_by_teacher_id, marked_at, created_at, updated_at, created_by, updated_by
		FROM lessonattendance
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list lessonattendances: %v", err)
	}
	defer rows.Close()

	entities := []*pb.LessonAttendance{}
	for rows.Next() {
		entity, err := scanLessonAttendance(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan lessonattendance: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating lessonattendances: %v", err)
	}

	return &pb.ListLessonAttendanceResponse{
		LessonAttendance: entities,
		Total:            total,
		Page:             page,
		PageSize:         pageSize,
	}, nil
}
