package handler

import (
	"context"
	"database/sql"
	"fmt"
	commonpb "github.com/thaily/lms/proto/common"
	pb "github.com/thaily/lms/proto/submission"
	"github.com/thaily/lms/src/service/pkg/helper"
	"github.com/thaily/lms/src/service/pkg/logger"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// scanExamAttempt reads a single row into a *pb.ExamAttempt.
// Shared by Create/Update inline-select and the List handler.
func scanExamAttempt(scanner interface{ Scan(...interface{}) error }) (*pb.ExamAttempt, error) {
	var entity pb.ExamAttempt
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var StatusStr string
	var StartedAtTime sql.NullTime
	var SubmittedAtTime sql.NullTime
	var GradedAtTime sql.NullTime
	var AssignmentIdNull sql.NullInt64
	var AutoScoreNull sql.NullFloat64
	var ManualScoreNull sql.NullFloat64
	var TotalScoreNull sql.NullFloat64
	var GradedByTeacherIdNull sql.NullInt64

	err := scanner.Scan(
		&entity.Id,
		&entity.ExamId,
		&entity.StudentId,
		&AssignmentIdNull,
		&StartedAtTime,
		&SubmittedAtTime,
		&AutoScoreNull,
		&ManualScoreNull,
		&TotalScoreNull,
		&StatusStr,
		&GradedByTeacherIdNull,
		&GradedAtTime,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch StatusStr {
	case "attempt_status_unspecified":
		entity.Status = pb.AttemptStatus_ATTEMPT_STATUS_UNSPECIFIED
	case "in_progress":
		entity.Status = pb.AttemptStatus_IN_PROGRESS
	case "submitted":
		entity.Status = pb.AttemptStatus_SUBMITTED
	case "graded":
		entity.Status = pb.AttemptStatus_GRADED
	default:
		entity.Status = pb.AttemptStatus_ATTEMPT_STATUS_UNSPECIFIED
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
	if StartedAtTime.Valid {
		entity.StartedAt = timestamppb.New(StartedAtTime.Time)
	}
	if SubmittedAtTime.Valid {
		entity.SubmittedAt = timestamppb.New(SubmittedAtTime.Time)
	}
	if GradedAtTime.Valid {
		entity.GradedAt = timestamppb.New(GradedAtTime.Time)
	}
	if AssignmentIdNull.Valid {
		val := uint64(AssignmentIdNull.Int64)
		entity.AssignmentId = &val
	}
	if AutoScoreNull.Valid {
		val := AutoScoreNull.Float64
		entity.AutoScore = &val
	}
	if ManualScoreNull.Valid {
		val := ManualScoreNull.Float64
		entity.ManualScore = &val
	}
	if TotalScoreNull.Valid {
		val := TotalScoreNull.Float64
		entity.TotalScore = &val
	}
	if GradedByTeacherIdNull.Valid {
		val := uint64(GradedByTeacherIdNull.Int64)
		entity.GradedByTeacherId = &val
	}

	return &entity, nil
}

// buildExamAttemptWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in examattempt_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildExamAttemptWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, ExamAttemptFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, ExamAttemptFilterableFields)
	return clause, args, err
}

// CreateExamAttempt creates a new ExamAttempt record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateExamAttempt(ctx context.Context, req *pb.CreateExamAttemptRequest) (*pb.CreateExamAttemptResponse, error) {
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
	// Optional uint64: AssignmentId
	var AssignmentId interface{}
	if req.AssignmentId != nil {
		AssignmentId = *req.AssignmentId
	}
	// Optional timestamp: SubmittedAt
	var SubmittedAt interface{}
	if req.SubmittedAt != nil {
		SubmittedAt = req.SubmittedAt.AsTime()
	}
	// Optional float64: AutoScore
	var AutoScore interface{}
	if req.AutoScore != nil {
		AutoScore = *req.AutoScore
	}
	// Optional float64: ManualScore
	var ManualScore interface{}
	if req.ManualScore != nil {
		ManualScore = *req.ManualScore
	}
	// Optional float64: TotalScore
	var TotalScore interface{}
	if req.TotalScore != nil {
		TotalScore = *req.TotalScore
	}
	// Optional uint64: GradedByTeacherId
	var GradedByTeacherId interface{}
	if req.GradedByTeacherId != nil {
		GradedByTeacherId = *req.GradedByTeacherId
	}
	// Optional timestamp: GradedAt
	var GradedAt interface{}
	if req.GradedAt != nil {
		GradedAt = req.GradedAt.AsTime()
	}

	// Convert Status enum to string
	StatusValue := pb.AttemptStatus_ATTEMPT_STATUS_UNSPECIFIED

	StatusValue = req.Status
	StatusStr := "attempt_status_unspecified"
	switch StatusValue {
	case pb.AttemptStatus_ATTEMPT_STATUS_UNSPECIFIED:
		StatusStr = "attempt_status_unspecified"
	case pb.AttemptStatus_IN_PROGRESS:
		StatusStr = "in_progress"
	case pb.AttemptStatus_SUBMITTED:
		StatusStr = "submitted"
	case pb.AttemptStatus_GRADED:
		StatusStr = "graded"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO examattempt (id, exam_id, student_id, assignment_id, started_at, submitted_at, auto_score, manual_score, total_score, status, graded_by_teacher_id, graded_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.ExamId,
		req.StudentId,
		AssignmentId,
		req.StartedAt.AsTime(),
		SubmittedAt,
		AutoScore,
		ManualScore,
		TotalScore,
		StatusStr,
		GradedByTeacherId,
		GradedAt,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "examattempt already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create examattempt: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetExamAttempt call).
	selectQuery := `
		SELECT id, exam_id, student_id, assignment_id, started_at, submitted_at, auto_score, manual_score, total_score, status, graded_by_teacher_id, graded_at, created_at, updated_at, created_by, updated_by
		FROM examattempt
		WHERE id = ?
	`
	entity, err := scanExamAttempt(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created examattempt: %v", err)
	}

	return &pb.CreateExamAttemptResponse{
		ExamAttempt: entity,
	}, nil
}

// UpdateExamAttempt applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateExamAttempt(ctx context.Context, req *pb.UpdateExamAttemptRequest) (*pb.UpdateExamAttemptResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildExamAttemptWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: SubmittedAt
	if req.SubmittedAt != nil {
		updateFields = append(updateFields, "submitted_at = ?")
		args = append(args, req.SubmittedAt.AsTime())

	}
	// Optional field: AutoScore
	if req.AutoScore != nil {
		updateFields = append(updateFields, "auto_score = ?")
		args = append(args, *req.AutoScore)

	}
	// Optional field: ManualScore
	if req.ManualScore != nil {
		updateFields = append(updateFields, "manual_score = ?")
		args = append(args, *req.ManualScore)

	}
	// Optional field: TotalScore
	if req.TotalScore != nil {
		updateFields = append(updateFields, "total_score = ?")
		args = append(args, *req.TotalScore)

	}
	// Optional field: Status
	if req.Status != nil {
		updateFields = append(updateFields, "status = ?")
		StatusStr := "attempt_status_unspecified"
		switch *req.Status {
		case pb.AttemptStatus_ATTEMPT_STATUS_UNSPECIFIED:
			StatusStr = "attempt_status_unspecified"
		case pb.AttemptStatus_IN_PROGRESS:
			StatusStr = "in_progress"
		case pb.AttemptStatus_SUBMITTED:
			StatusStr = "submitted"
		case pb.AttemptStatus_GRADED:
			StatusStr = "graded"
		}
		args = append(args, StatusStr)

	}
	// Optional field: GradedByTeacherId
	if req.GradedByTeacherId != nil {
		updateFields = append(updateFields, "graded_by_teacher_id = ?")
		args = append(args, *req.GradedByTeacherId)

	}
	// Optional field: GradedAt
	if req.GradedAt != nil {
		updateFields = append(updateFields, "graded_at = ?")
		args = append(args, req.GradedAt.AsTime())

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

	query := fmt.Sprintf(`UPDATE examattempt SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update examattempt: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, exam_id, student_id, assignment_id, started_at, submitted_at, auto_score, manual_score, total_score, status, graded_by_teacher_id, graded_at, created_at, updated_at, created_by, updated_by FROM examattempt %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated examattempts: %v", err)
	}
	defer rows.Close()

	entities := []*pb.ExamAttempt{}
	for rows.Next() {
		entity, err := scanExamAttempt(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan examattempt: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating examattempts: %v", err)
	}

	return &pb.UpdateExamAttemptResponse{
		ExamAttempt:   entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteExamAttempt deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteExamAttempt(ctx context.Context, req *pb.DeleteExamAttemptRequest) (*pb.DeleteExamAttemptResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildExamAttemptWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM examattempt %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete examattempt: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteExamAttemptResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListExamAttempt lists ExamAttempts with pagination and filtering
func (h *Handler) ListExamAttempt(ctx context.Context, req *pb.ListExamAttemptRequest) (*pb.ListExamAttemptResponse, error) {
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
		whereClause, args, err = buildExamAttemptWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM examattempt %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count examattempts: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, exam_id, student_id, assignment_id, started_at, submitted_at, auto_score, manual_score, total_score, status, graded_by_teacher_id, graded_at, created_at, updated_at, created_by, updated_by
		FROM examattempt
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list examattempts: %v", err)
	}
	defer rows.Close()

	entities := []*pb.ExamAttempt{}
	for rows.Next() {
		entity, err := scanExamAttempt(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan examattempt: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating examattempts: %v", err)
	}

	return &pb.ListExamAttemptResponse{
		ExamAttempt: entities,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}
