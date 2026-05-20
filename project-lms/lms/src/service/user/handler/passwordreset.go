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

// scanPasswordReset reads a single row into a *pb.PasswordReset.
// Shared by Create/Update inline-select and the List handler.
func scanPasswordReset(scanner interface{ Scan(...interface{}) error }) (*pb.PasswordReset, error) {
	var entity pb.PasswordReset
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var ExpiresAtTime sql.NullTime
	var UsedAtTime sql.NullTime

	err := scanner.Scan(
		&entity.Id,
		&entity.UserId,
		&entity.TokenHash,
		&ExpiresAtTime,
		&UsedAtTime,
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
	if ExpiresAtTime.Valid {
		entity.ExpiresAt = timestamppb.New(ExpiresAtTime.Time)
	}
	if UsedAtTime.Valid {
		entity.UsedAt = timestamppb.New(UsedAtTime.Time)
	}

	return &entity, nil
}

// buildPasswordResetWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in passwordreset_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildPasswordResetWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, PasswordResetFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, PasswordResetFilterableFields)
	return clause, args, err
}

// CreatePasswordReset creates a new PasswordReset record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreatePasswordReset(ctx context.Context, req *pb.CreatePasswordResetRequest) (*pb.CreatePasswordResetResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.TokenHash == "" {
		return nil, status.Error(codes.InvalidArgument, "token_hash is required")
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
	// Optional timestamp: UsedAt
	var UsedAt interface{}
	if req.UsedAt != nil {
		UsedAt = req.UsedAt.AsTime()
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO passwordreset (id, user_id, token_hash, expires_at, used_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.UserId,
		req.TokenHash,
		req.ExpiresAt.AsTime(),
		UsedAt,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "passwordreset already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create passwordreset: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetPasswordReset call).
	selectQuery := `
		SELECT id, user_id, token_hash, expires_at, used_at, created_at, updated_at, created_by, updated_by
		FROM passwordreset
		WHERE id = ?
	`
	entity, err := scanPasswordReset(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created passwordreset: %v", err)
	}

	return &pb.CreatePasswordResetResponse{
		PasswordReset: entity,
	}, nil
}

// UpdatePasswordReset applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdatePasswordReset(ctx context.Context, req *pb.UpdatePasswordResetRequest) (*pb.UpdatePasswordResetResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildPasswordResetWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: UsedAt
	if req.UsedAt != nil {
		updateFields = append(updateFields, "used_at = ?")
		args = append(args, req.UsedAt.AsTime())

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

	query := fmt.Sprintf(`UPDATE passwordreset SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update passwordreset: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, user_id, token_hash, expires_at, used_at, created_at, updated_at, created_by, updated_by FROM passwordreset %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated passwordresets: %v", err)
	}
	defer rows.Close()

	entities := []*pb.PasswordReset{}
	for rows.Next() {
		entity, err := scanPasswordReset(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan passwordreset: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating passwordresets: %v", err)
	}

	return &pb.UpdatePasswordResetResponse{
		PasswordReset: entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeletePasswordReset deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeletePasswordReset(ctx context.Context, req *pb.DeletePasswordResetRequest) (*pb.DeletePasswordResetResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildPasswordResetWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM passwordreset %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete passwordreset: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeletePasswordResetResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListPasswordReset lists PasswordResets with pagination and filtering
func (h *Handler) ListPasswordReset(ctx context.Context, req *pb.ListPasswordResetRequest) (*pb.ListPasswordResetResponse, error) {
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
		whereClause, args, err = buildPasswordResetWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM passwordreset %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count passwordresets: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, user_id, token_hash, expires_at, used_at, created_at, updated_at, created_by, updated_by
		FROM passwordreset
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list passwordresets: %v", err)
	}
	defer rows.Close()

	entities := []*pb.PasswordReset{}
	for rows.Next() {
		entity, err := scanPasswordReset(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan passwordreset: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating passwordresets: %v", err)
	}

	return &pb.ListPasswordResetResponse{
		PasswordReset: entities,
		Total:         total,
		Page:          page,
		PageSize:      pageSize,
	}, nil
}
