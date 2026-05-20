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

// scanLesson reads a single row into a *pb.Lesson.
// Shared by Create/Update inline-select and the List handler.
func scanLesson(scanner interface{ Scan(...interface{}) error }) (*pb.Lesson, error) {
	var entity pb.Lesson
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var ModeStr string
	var StatusStr string
	var StartAtTime sql.NullTime
	var EndAtTime sql.NullTime
	var DescriptionNull sql.NullString
	var LocationNull sql.NullString
	var MeetingUrlNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.ClassId,
		&entity.Title,
		&DescriptionNull,
		&StartAtTime,
		&EndAtTime,
		&ModeStr,
		&LocationNull,
		&MeetingUrlNull,
		&StatusStr,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch ModeStr {
	case "lesson_mode_unspecified":
		entity.Mode = pb.LessonMode_LESSON_MODE_UNSPECIFIED
	case "offline":
		entity.Mode = pb.LessonMode_OFFLINE
	case "online":
		entity.Mode = pb.LessonMode_ONLINE
	case "hybrid":
		entity.Mode = pb.LessonMode_HYBRID
	default:
		entity.Mode = pb.LessonMode_LESSON_MODE_UNSPECIFIED
	}
	switch StatusStr {
	case "lesson_status_unspecified":
		entity.Status = pb.LessonStatus_LESSON_STATUS_UNSPECIFIED
	case "scheduled":
		entity.Status = pb.LessonStatus_SCHEDULED
	case "ongoing":
		entity.Status = pb.LessonStatus_ONGOING
	case "done":
		entity.Status = pb.LessonStatus_DONE
	case "cancelled":
		entity.Status = pb.LessonStatus_CANCELLED
	default:
		entity.Status = pb.LessonStatus_LESSON_STATUS_UNSPECIFIED
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
	if StartAtTime.Valid {
		entity.StartAt = timestamppb.New(StartAtTime.Time)
	}
	if EndAtTime.Valid {
		entity.EndAt = timestamppb.New(EndAtTime.Time)
	}
	if DescriptionNull.Valid {
		val := DescriptionNull.String
		entity.Description = &val
	}
	if LocationNull.Valid {
		val := LocationNull.String
		entity.Location = &val
	}
	if MeetingUrlNull.Valid {
		val := MeetingUrlNull.String
		entity.MeetingUrl = &val
	}

	return &entity, nil
}

// buildLessonWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in lesson_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildLessonWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, LessonFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, LessonFilterableFields)
	return clause, args, err
}

// CreateLesson creates a new Lesson record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateLesson(ctx context.Context, req *pb.CreateLessonRequest) (*pb.CreateLessonResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
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
	// Optional string: Description
	var Description interface{}
	if req.Description != nil {
		Description = *req.Description
	}
	// Optional string: Location
	var Location interface{}
	if req.Location != nil {
		Location = *req.Location
	}
	// Optional string: MeetingUrl
	var MeetingUrl interface{}
	if req.MeetingUrl != nil {
		MeetingUrl = *req.MeetingUrl
	}

	// Convert Mode enum to string
	ModeValue := pb.LessonMode_LESSON_MODE_UNSPECIFIED

	ModeValue = req.Mode
	ModeStr := "lesson_mode_unspecified"
	switch ModeValue {
	case pb.LessonMode_LESSON_MODE_UNSPECIFIED:
		ModeStr = "lesson_mode_unspecified"
	case pb.LessonMode_OFFLINE:
		ModeStr = "offline"
	case pb.LessonMode_ONLINE:
		ModeStr = "online"
	case pb.LessonMode_HYBRID:
		ModeStr = "hybrid"
	}
	// Convert Status enum to string
	StatusValue := pb.LessonStatus_LESSON_STATUS_UNSPECIFIED

	StatusValue = req.Status
	StatusStr := "lesson_status_unspecified"
	switch StatusValue {
	case pb.LessonStatus_LESSON_STATUS_UNSPECIFIED:
		StatusStr = "lesson_status_unspecified"
	case pb.LessonStatus_SCHEDULED:
		StatusStr = "scheduled"
	case pb.LessonStatus_ONGOING:
		StatusStr = "ongoing"
	case pb.LessonStatus_DONE:
		StatusStr = "done"
	case pb.LessonStatus_CANCELLED:
		StatusStr = "cancelled"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO lesson (id, class_id, title, description, start_at, end_at, mode, location, meeting_url, status, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.ClassId,
		req.Title,
		Description,
		req.StartAt.AsTime(),
		req.EndAt.AsTime(),
		ModeStr,
		Location,
		MeetingUrl,
		StatusStr,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "lesson already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create lesson: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetLesson call).
	selectQuery := `
		SELECT id, class_id, title, description, start_at, end_at, mode, location, meeting_url, status, created_at, updated_at, created_by, updated_by
		FROM lesson
		WHERE id = ?
	`
	entity, err := scanLesson(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created lesson: %v", err)
	}

	return &pb.CreateLessonResponse{
		Lesson: entity,
	}, nil
}

// UpdateLesson applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateLesson(ctx context.Context, req *pb.UpdateLessonRequest) (*pb.UpdateLessonResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildLessonWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Title
	if req.Title != nil {
		updateFields = append(updateFields, "title = ?")
		args = append(args, *req.Title)

	}
	// Optional field: Description
	if req.Description != nil {
		updateFields = append(updateFields, "description = ?")
		args = append(args, *req.Description)

	}
	// Optional field: StartAt
	if req.StartAt != nil {
		updateFields = append(updateFields, "start_at = ?")
		args = append(args, req.StartAt.AsTime())

	}
	// Optional field: EndAt
	if req.EndAt != nil {
		updateFields = append(updateFields, "end_at = ?")
		args = append(args, req.EndAt.AsTime())

	}
	// Optional field: Mode
	if req.Mode != nil {
		updateFields = append(updateFields, "mode = ?")
		ModeStr := "lesson_mode_unspecified"
		switch *req.Mode {
		case pb.LessonMode_LESSON_MODE_UNSPECIFIED:
			ModeStr = "lesson_mode_unspecified"
		case pb.LessonMode_OFFLINE:
			ModeStr = "offline"
		case pb.LessonMode_ONLINE:
			ModeStr = "online"
		case pb.LessonMode_HYBRID:
			ModeStr = "hybrid"
		}
		args = append(args, ModeStr)

	}
	// Optional field: Location
	if req.Location != nil {
		updateFields = append(updateFields, "location = ?")
		args = append(args, *req.Location)

	}
	// Optional field: MeetingUrl
	if req.MeetingUrl != nil {
		updateFields = append(updateFields, "meeting_url = ?")
		args = append(args, *req.MeetingUrl)

	}
	// Optional field: Status
	if req.Status != nil {
		updateFields = append(updateFields, "status = ?")
		StatusStr := "lesson_status_unspecified"
		switch *req.Status {
		case pb.LessonStatus_LESSON_STATUS_UNSPECIFIED:
			StatusStr = "lesson_status_unspecified"
		case pb.LessonStatus_SCHEDULED:
			StatusStr = "scheduled"
		case pb.LessonStatus_ONGOING:
			StatusStr = "ongoing"
		case pb.LessonStatus_DONE:
			StatusStr = "done"
		case pb.LessonStatus_CANCELLED:
			StatusStr = "cancelled"
		}
		args = append(args, StatusStr)

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

	query := fmt.Sprintf(`UPDATE lesson SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update lesson: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, class_id, title, description, start_at, end_at, mode, location, meeting_url, status, created_at, updated_at, created_by, updated_by FROM lesson %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated lessons: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Lesson{}
	for rows.Next() {
		entity, err := scanLesson(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan lesson: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating lessons: %v", err)
	}

	return &pb.UpdateLessonResponse{
		Lesson:        entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteLesson deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteLesson(ctx context.Context, req *pb.DeleteLessonRequest) (*pb.DeleteLessonResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildLessonWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM lesson %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete lesson: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteLessonResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListLesson lists Lessons with pagination and filtering
func (h *Handler) ListLesson(ctx context.Context, req *pb.ListLessonRequest) (*pb.ListLessonResponse, error) {
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
		whereClause, args, err = buildLessonWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM lesson %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count lessons: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, class_id, title, description, start_at, end_at, mode, location, meeting_url, status, created_at, updated_at, created_by, updated_by
		FROM lesson
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list lessons: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Lesson{}
	for rows.Next() {
		entity, err := scanLesson(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan lesson: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating lessons: %v", err)
	}

	return &pb.ListLessonResponse{
		Lesson:   entities,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
