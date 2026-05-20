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

// scanAttemptAnswer reads a single row into a *pb.AttemptAnswer.
// Shared by Create/Update inline-select and the List handler.
func scanAttemptAnswer(scanner interface{ Scan(...interface{}) error }) (*pb.AttemptAnswer, error) {
	var entity pb.AttemptAnswer
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var GradedAtTime sql.NullTime
	var SelectedChoiceIdsNull sql.NullString
	var TextAnswerNull sql.NullString
	var UploadedFileKeyNull sql.NullString
	var AutoScoreNull sql.NullFloat64
	var ManualScoreNull sql.NullFloat64
	var TeacherCommentNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.AttemptId,
		&entity.QuestionId,
		&SelectedChoiceIdsNull,
		&TextAnswerNull,
		&UploadedFileKeyNull,
		&AutoScoreNull,
		&ManualScoreNull,
		&TeacherCommentNull,
		&GradedAtTime,
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
	if GradedAtTime.Valid {
		entity.GradedAt = timestamppb.New(GradedAtTime.Time)
	}
	if SelectedChoiceIdsNull.Valid {
		val := SelectedChoiceIdsNull.String
		entity.SelectedChoiceIds = &val
	}
	if TextAnswerNull.Valid {
		val := TextAnswerNull.String
		entity.TextAnswer = &val
	}
	if UploadedFileKeyNull.Valid {
		val := UploadedFileKeyNull.String
		entity.UploadedFileKey = &val
	}
	if AutoScoreNull.Valid {
		val := AutoScoreNull.Float64
		entity.AutoScore = &val
	}
	if ManualScoreNull.Valid {
		val := ManualScoreNull.Float64
		entity.ManualScore = &val
	}
	if TeacherCommentNull.Valid {
		val := TeacherCommentNull.String
		entity.TeacherComment = &val
	}

	return &entity, nil
}

// buildAttemptAnswerWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in attemptanswer_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildAttemptAnswerWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, AttemptAnswerFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, AttemptAnswerFilterableFields)
	return clause, args, err
}

// CreateAttemptAnswer creates a new AttemptAnswer record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateAttemptAnswer(ctx context.Context, req *pb.CreateAttemptAnswerRequest) (*pb.CreateAttemptAnswerResponse, error) {
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
	// Optional string: SelectedChoiceIds
	var SelectedChoiceIds interface{}
	if req.SelectedChoiceIds != nil {
		SelectedChoiceIds = *req.SelectedChoiceIds
	}
	// Optional string: TextAnswer
	var TextAnswer interface{}
	if req.TextAnswer != nil {
		TextAnswer = *req.TextAnswer
	}
	// Optional string: UploadedFileKey
	var UploadedFileKey interface{}
	if req.UploadedFileKey != nil {
		UploadedFileKey = *req.UploadedFileKey
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
	// Optional string: TeacherComment
	var TeacherComment interface{}
	if req.TeacherComment != nil {
		TeacherComment = *req.TeacherComment
	}
	// Optional timestamp: GradedAt
	var GradedAt interface{}
	if req.GradedAt != nil {
		GradedAt = req.GradedAt.AsTime()
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO attemptanswer (id, attempt_id, question_id, selected_choice_ids, text_answer, uploaded_file_key, auto_score, manual_score, teacher_comment, graded_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.AttemptId,
		req.QuestionId,
		SelectedChoiceIds,
		TextAnswer,
		UploadedFileKey,
		AutoScore,
		ManualScore,
		TeacherComment,
		GradedAt,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "attemptanswer already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create attemptanswer: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetAttemptAnswer call).
	selectQuery := `
		SELECT id, attempt_id, question_id, selected_choice_ids, text_answer, uploaded_file_key, auto_score, manual_score, teacher_comment, graded_at, created_at, updated_at, created_by, updated_by
		FROM attemptanswer
		WHERE id = ?
	`
	entity, err := scanAttemptAnswer(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created attemptanswer: %v", err)
	}

	return &pb.CreateAttemptAnswerResponse{
		AttemptAnswer: entity,
	}, nil
}

// UpdateAttemptAnswer applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateAttemptAnswer(ctx context.Context, req *pb.UpdateAttemptAnswerRequest) (*pb.UpdateAttemptAnswerResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildAttemptAnswerWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: SelectedChoiceIds
	if req.SelectedChoiceIds != nil {
		updateFields = append(updateFields, "selected_choice_ids = ?")
		args = append(args, *req.SelectedChoiceIds)

	}
	// Optional field: TextAnswer
	if req.TextAnswer != nil {
		updateFields = append(updateFields, "text_answer = ?")
		args = append(args, *req.TextAnswer)

	}
	// Optional field: UploadedFileKey
	if req.UploadedFileKey != nil {
		updateFields = append(updateFields, "uploaded_file_key = ?")
		args = append(args, *req.UploadedFileKey)

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
	// Optional field: TeacherComment
	if req.TeacherComment != nil {
		updateFields = append(updateFields, "teacher_comment = ?")
		args = append(args, *req.TeacherComment)

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

	query := fmt.Sprintf(`UPDATE attemptanswer SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update attemptanswer: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, attempt_id, question_id, selected_choice_ids, text_answer, uploaded_file_key, auto_score, manual_score, teacher_comment, graded_at, created_at, updated_at, created_by, updated_by FROM attemptanswer %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated attemptanswers: %v", err)
	}
	defer rows.Close()

	entities := []*pb.AttemptAnswer{}
	for rows.Next() {
		entity, err := scanAttemptAnswer(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan attemptanswer: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating attemptanswers: %v", err)
	}

	return &pb.UpdateAttemptAnswerResponse{
		AttemptAnswer: entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteAttemptAnswer deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteAttemptAnswer(ctx context.Context, req *pb.DeleteAttemptAnswerRequest) (*pb.DeleteAttemptAnswerResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildAttemptAnswerWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM attemptanswer %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete attemptanswer: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteAttemptAnswerResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListAttemptAnswer lists AttemptAnswers with pagination and filtering
func (h *Handler) ListAttemptAnswer(ctx context.Context, req *pb.ListAttemptAnswerRequest) (*pb.ListAttemptAnswerResponse, error) {
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
		whereClause, args, err = buildAttemptAnswerWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM attemptanswer %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count attemptanswers: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, attempt_id, question_id, selected_choice_ids, text_answer, uploaded_file_key, auto_score, manual_score, teacher_comment, graded_at, created_at, updated_at, created_by, updated_by
		FROM attemptanswer
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list attemptanswers: %v", err)
	}
	defer rows.Close()

	entities := []*pb.AttemptAnswer{}
	for rows.Next() {
		entity, err := scanAttemptAnswer(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan attemptanswer: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating attemptanswers: %v", err)
	}

	return &pb.ListAttemptAnswerResponse{
		AttemptAnswer: entities,
		Total:         total,
		Page:          page,
		PageSize:      pageSize,
	}, nil
}
