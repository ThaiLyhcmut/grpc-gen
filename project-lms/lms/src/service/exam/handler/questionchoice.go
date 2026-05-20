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

// scanQuestionChoice reads a single row into a *pb.QuestionChoice.
// Shared by Create/Update inline-select and the List handler.
func scanQuestionChoice(scanner interface{ Scan(...interface{}) error }) (*pb.QuestionChoice, error) {
	var entity pb.QuestionChoice
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.QuestionId,
		&entity.Position,
		&entity.Content,
		&entity.IsCorrect,
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

	return &entity, nil
}

// buildQuestionChoiceWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in questionchoice_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildQuestionChoiceWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, QuestionChoiceFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, QuestionChoiceFilterableFields)
	return clause, args, err
}

// CreateQuestionChoice creates a new QuestionChoice record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateQuestionChoice(ctx context.Context, req *pb.CreateQuestionChoiceRequest) (*pb.CreateQuestionChoiceResponse, error) {
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

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO questionchoice (id, question_id, position, content, is_correct, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.QuestionId,
		req.Position,
		req.Content,
		req.IsCorrect,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "questionchoice already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create questionchoice: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetQuestionChoice call).
	selectQuery := `
		SELECT id, question_id, position, content, is_correct, created_at, updated_at, created_by, updated_by
		FROM questionchoice
		WHERE id = ?
	`
	entity, err := scanQuestionChoice(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created questionchoice: %v", err)
	}

	return &pb.CreateQuestionChoiceResponse{
		QuestionChoice: entity,
	}, nil
}

// UpdateQuestionChoice applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateQuestionChoice(ctx context.Context, req *pb.UpdateQuestionChoiceRequest) (*pb.UpdateQuestionChoiceResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildQuestionChoiceWhere(req.GetFilters())
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
	// Optional field: Content
	if req.Content != nil {
		updateFields = append(updateFields, "content = ?")
		args = append(args, *req.Content)

	}
	// Optional field: IsCorrect
	if req.IsCorrect != nil {
		updateFields = append(updateFields, "is_correct = ?")
		args = append(args, *req.IsCorrect)

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

	query := fmt.Sprintf(`UPDATE questionchoice SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update questionchoice: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, question_id, position, content, is_correct, created_at, updated_at, created_by, updated_by FROM questionchoice %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated questionchoices: %v", err)
	}
	defer rows.Close()

	entities := []*pb.QuestionChoice{}
	for rows.Next() {
		entity, err := scanQuestionChoice(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan questionchoice: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating questionchoices: %v", err)
	}

	return &pb.UpdateQuestionChoiceResponse{
		QuestionChoice: entities,
		AffectedCount:  int32(affected),
	}, nil
}

// DeleteQuestionChoice deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteQuestionChoice(ctx context.Context, req *pb.DeleteQuestionChoiceRequest) (*pb.DeleteQuestionChoiceResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildQuestionChoiceWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM questionchoice %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete questionchoice: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteQuestionChoiceResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListQuestionChoice lists QuestionChoices with pagination and filtering
func (h *Handler) ListQuestionChoice(ctx context.Context, req *pb.ListQuestionChoiceRequest) (*pb.ListQuestionChoiceResponse, error) {
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
		whereClause, args, err = buildQuestionChoiceWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM questionchoice %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count questionchoices: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, question_id, position, content, is_correct, created_at, updated_at, created_by, updated_by
		FROM questionchoice
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list questionchoices: %v", err)
	}
	defer rows.Close()

	entities := []*pb.QuestionChoice{}
	for rows.Next() {
		entity, err := scanQuestionChoice(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan questionchoice: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating questionchoices: %v", err)
	}

	return &pb.ListQuestionChoiceResponse{
		QuestionChoice: entities,
		Total:          total,
		Page:           page,
		PageSize:       pageSize,
	}, nil
}
