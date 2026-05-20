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

// scanRefreshToken reads a single row into a *pb.RefreshToken.
// Shared by Create/Update inline-select and the List handler.
func scanRefreshToken(scanner interface{ Scan(...interface{}) error }) (*pb.RefreshToken, error) {
	var entity pb.RefreshToken
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var ExpiresAtTime sql.NullTime
	var RevokedAtTime sql.NullTime
	var UserAgentNull sql.NullString
	var IpAddressNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.UserId,
		&entity.TokenHash,
		&UserAgentNull,
		&IpAddressNull,
		&ExpiresAtTime,
		&RevokedAtTime,
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
	if RevokedAtTime.Valid {
		entity.RevokedAt = timestamppb.New(RevokedAtTime.Time)
	}
	if UserAgentNull.Valid {
		val := UserAgentNull.String
		entity.UserAgent = &val
	}
	if IpAddressNull.Valid {
		val := IpAddressNull.String
		entity.IpAddress = &val
	}

	return &entity, nil
}

// buildRefreshTokenWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in refreshtoken_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildRefreshTokenWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, RefreshTokenFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, RefreshTokenFilterableFields)
	return clause, args, err
}

// CreateRefreshToken creates a new RefreshToken record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateRefreshToken(ctx context.Context, req *pb.CreateRefreshTokenRequest) (*pb.CreateRefreshTokenResponse, error) {
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
	// Optional string: UserAgent
	var UserAgent interface{}
	if req.UserAgent != nil {
		UserAgent = *req.UserAgent
	}
	// Optional string: IpAddress
	var IpAddress interface{}
	if req.IpAddress != nil {
		IpAddress = *req.IpAddress
	}
	// Optional timestamp: RevokedAt
	var RevokedAt interface{}
	if req.RevokedAt != nil {
		RevokedAt = req.RevokedAt.AsTime()
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO refreshtoken (id, user_id, token_hash, user_agent, ip_address, expires_at, revoked_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.UserId,
		req.TokenHash,
		UserAgent,
		IpAddress,
		req.ExpiresAt.AsTime(),
		RevokedAt,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "refreshtoken already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create refreshtoken: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetRefreshToken call).
	selectQuery := `
		SELECT id, user_id, token_hash, user_agent, ip_address, expires_at, revoked_at, created_at, updated_at, created_by, updated_by
		FROM refreshtoken
		WHERE id = ?
	`
	entity, err := scanRefreshToken(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created refreshtoken: %v", err)
	}

	return &pb.CreateRefreshTokenResponse{
		RefreshToken: entity,
	}, nil
}

// UpdateRefreshToken applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateRefreshToken(ctx context.Context, req *pb.UpdateRefreshTokenRequest) (*pb.UpdateRefreshTokenResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildRefreshTokenWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: RevokedAt
	if req.RevokedAt != nil {
		updateFields = append(updateFields, "revoked_at = ?")
		args = append(args, req.RevokedAt.AsTime())

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

	query := fmt.Sprintf(`UPDATE refreshtoken SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update refreshtoken: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, user_id, token_hash, user_agent, ip_address, expires_at, revoked_at, created_at, updated_at, created_by, updated_by FROM refreshtoken %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated refreshtokens: %v", err)
	}
	defer rows.Close()

	entities := []*pb.RefreshToken{}
	for rows.Next() {
		entity, err := scanRefreshToken(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan refreshtoken: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating refreshtokens: %v", err)
	}

	return &pb.UpdateRefreshTokenResponse{
		RefreshToken:  entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteRefreshToken deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteRefreshToken(ctx context.Context, req *pb.DeleteRefreshTokenRequest) (*pb.DeleteRefreshTokenResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildRefreshTokenWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM refreshtoken %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete refreshtoken: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteRefreshTokenResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListRefreshToken lists RefreshTokens with pagination and filtering
func (h *Handler) ListRefreshToken(ctx context.Context, req *pb.ListRefreshTokenRequest) (*pb.ListRefreshTokenResponse, error) {
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
		whereClause, args, err = buildRefreshTokenWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM refreshtoken %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count refreshtokens: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, user_id, token_hash, user_agent, ip_address, expires_at, revoked_at, created_at, updated_at, created_by, updated_by
		FROM refreshtoken
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list refreshtokens: %v", err)
	}
	defer rows.Close()

	entities := []*pb.RefreshToken{}
	for rows.Next() {
		entity, err := scanRefreshToken(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan refreshtoken: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating refreshtokens: %v", err)
	}

	return &pb.ListRefreshTokenResponse{
		RefreshToken: entities,
		Total:        total,
		Page:         page,
		PageSize:     pageSize,
	}, nil
}
