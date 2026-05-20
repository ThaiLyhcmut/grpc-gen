package handler

import (
	"context"
	"database/sql"
	"fmt"
	commonpb "github.com/thaily/lms/proto/common"
	pb "github.com/thaily/lms/proto/exam"
	"github.com/thaily/lms/src/service/pkg/helper"
	"github.com/thaily/lms/src/service/pkg/logger"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// scanQuestion reads a single row into a *pb.Question.
// Shared by Create/Update inline-select and the List handler.
func scanQuestion(scanner interface{ Scan(...interface{}) error }) (*pb.Question, error) {
	var entity pb.Question
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var TypeStr string
	var ImageKeyNull sql.NullString
	var ExplanationNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.ExamId,
		&entity.Position,
		&TypeStr,
		&entity.Content,
		&ImageKeyNull,
		&entity.Points,
		&ExplanationNull,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch TypeStr {
	case "question_type_unspecified":
		entity.Type = pb.QuestionType_QUESTION_TYPE_UNSPECIFIED
	case "single":
		entity.Type = pb.QuestionType_SINGLE
	case "multiple":
		entity.Type = pb.QuestionType_MULTIPLE
	case "short_answer":
		entity.Type = pb.QuestionType_SHORT_ANSWER
	case "essay":
		entity.Type = pb.QuestionType_ESSAY
	default:
		entity.Type = pb.QuestionType_QUESTION_TYPE_UNSPECIFIED
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
	if ImageKeyNull.Valid {
		val := ImageKeyNull.String
		entity.ImageKey = &val
	}
	if ExplanationNull.Valid {
		val := ExplanationNull.String
		entity.Explanation = &val
	}

	return &entity, nil
}

// buildQuestionWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in question_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildQuestionWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, QuestionFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, QuestionFilterableFields)
	return clause, args, err
}

// CreateQuestion creates a new Question record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateQuestion(ctx context.Context, req *pb.CreateQuestionRequest) (*pb.CreateQuestionResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.Content == "" {
		return nil, status.Error(codes.InvalidArgument, "content is required")
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
	// Optional string: ImageKey
	var ImageKey interface{}
	if req.ImageKey != nil {
		ImageKey = *req.ImageKey
	}
	// Optional string: Explanation
	var Explanation interface{}
	if req.Explanation != nil {
		Explanation = *req.Explanation
	}

	// Convert Type enum to string
	TypeValue := pb.QuestionType_QUESTION_TYPE_UNSPECIFIED

	TypeValue = req.Type
	TypeStr := "question_type_unspecified"
	switch TypeValue {
	case pb.QuestionType_QUESTION_TYPE_UNSPECIFIED:
		TypeStr = "question_type_unspecified"
	case pb.QuestionType_SINGLE:
		TypeStr = "single"
	case pb.QuestionType_MULTIPLE:
		TypeStr = "multiple"
	case pb.QuestionType_SHORT_ANSWER:
		TypeStr = "short_answer"
	case pb.QuestionType_ESSAY:
		TypeStr = "essay"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO question (id, exam_id, position, type, content, image_key, points, explanation, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.ExamId,
		req.Position,
		TypeStr,
		req.Content,
		ImageKey,
		req.Points,
		Explanation,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "question already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create question: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetQuestion call).
	selectQuery := `
		SELECT id, exam_id, position, type, content, image_key, points, explanation, created_at, updated_at, created_by, updated_by
		FROM question
		WHERE id = ?
	`
	entity, err := scanQuestion(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created question: %v", err)
	}

	return &pb.CreateQuestionResponse{
		Question: entity,
	}, nil
}

// UpdateQuestion applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateQuestion(ctx context.Context, req *pb.UpdateQuestionRequest) (*pb.UpdateQuestionResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildQuestionWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Position
	if req.Position != nil {
		updateFields = append(updateFields, "position = ?")
		args = append(args, *req.Position)

	}
	// Optional field: Type
	if req.Type != nil {
		updateFields = append(updateFields, "type = ?")
		TypeStr := "question_type_unspecified"
		switch *req.Type {
		case pb.QuestionType_QUESTION_TYPE_UNSPECIFIED:
			TypeStr = "question_type_unspecified"
		case pb.QuestionType_SINGLE:
			TypeStr = "single"
		case pb.QuestionType_MULTIPLE:
			TypeStr = "multiple"
		case pb.QuestionType_SHORT_ANSWER:
			TypeStr = "short_answer"
		case pb.QuestionType_ESSAY:
			TypeStr = "essay"
		}
		args = append(args, TypeStr)

	}
	// Optional field: Content
	if req.Content != nil {
		updateFields = append(updateFields, "content = ?")
		args = append(args, *req.Content)

	}
	// Optional field: ImageKey
	if req.ImageKey != nil {
		updateFields = append(updateFields, "image_key = ?")
		args = append(args, *req.ImageKey)

	}
	// Optional field: Points
	if req.Points != nil {
		updateFields = append(updateFields, "points = ?")
		args = append(args, *req.Points)

	}
	// Optional field: Explanation
	if req.Explanation != nil {
		updateFields = append(updateFields, "explanation = ?")
		args = append(args, *req.Explanation)

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

	query := fmt.Sprintf(`UPDATE question SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update question: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, exam_id, position, type, content, image_key, points, explanation, created_at, updated_at, created_by, updated_by FROM question %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated questions: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Question{}
	for rows.Next() {
		entity, err := scanQuestion(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan question: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating questions: %v", err)
	}

	return &pb.UpdateQuestionResponse{
		Question:      entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteQuestion deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteQuestion(ctx context.Context, req *pb.DeleteQuestionRequest) (*pb.DeleteQuestionResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildQuestionWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM question %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete question: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteQuestionResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListQuestion lists Questions with pagination and filtering
func (h *Handler) ListQuestion(ctx context.Context, req *pb.ListQuestionRequest) (*pb.ListQuestionResponse, error) {
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
		whereClause, args, err = buildQuestionWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM question %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count questions: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, exam_id, position, type, content, image_key, points, explanation, created_at, updated_at, created_by, updated_by
		FROM question
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list questions: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Question{}
	for rows.Next() {
		entity, err := scanQuestion(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan question: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating questions: %v", err)
	}

	return &pb.ListQuestionResponse{
		Question: entities,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
