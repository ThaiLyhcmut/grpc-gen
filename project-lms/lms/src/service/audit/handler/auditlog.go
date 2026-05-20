package handler

import (
	"context"
	"database/sql"
	"fmt"
	pb "github.com/thaily/lms/proto/audit"
	commonpb "github.com/thaily/lms/proto/common"
	"github.com/thaily/lms/src/service/pkg/helper"
	"github.com/thaily/lms/src/service/pkg/logger"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// scanAuditLog reads a single row into a *pb.AuditLog.
// Shared by Create/Update inline-select and the List handler.
func scanAuditLog(scanner interface{ Scan(...interface{}) error }) (*pb.AuditLog, error) {
	var entity pb.AuditLog
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var TargetTypeNull sql.NullString
	var TargetIdNull sql.NullInt64
	var MetaNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.ActorId,
		&entity.Action,
		&TargetTypeNull,
		&TargetIdNull,
		&MetaNull,
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
	if TargetTypeNull.Valid {
		val := TargetTypeNull.String
		entity.TargetType = &val
	}
	if TargetIdNull.Valid {
		val := uint64(TargetIdNull.Int64)
		entity.TargetId = &val
	}
	if MetaNull.Valid {
		val := MetaNull.String
		entity.Meta = &val
	}

	return &entity, nil
}

// buildAuditLogWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in auditlog_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildAuditLogWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, AuditLogFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, AuditLogFilterableFields)
	return clause, args, err
}

// CreateAuditLog creates a new AuditLog record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateAuditLog(ctx context.Context, req *pb.CreateAuditLogRequest) (*pb.CreateAuditLogResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.Action == "" {
		return nil, status.Error(codes.InvalidArgument, "action is required")
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
	// Optional string: TargetType
	var TargetType interface{}
	if req.TargetType != nil {
		TargetType = *req.TargetType
	}
	// Optional uint64: TargetId
	var TargetId interface{}
	if req.TargetId != nil {
		TargetId = *req.TargetId
	}
	// Optional string: Meta
	var Meta interface{}
	if req.Meta != nil {
		Meta = *req.Meta
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO auditlog (id, actor_id, action, target_type, target_id, meta, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.ActorId,
		req.Action,
		TargetType,
		TargetId,
		Meta,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "auditlog already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create auditlog: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetAuditLog call).
	selectQuery := `
		SELECT id, actor_id, action, target_type, target_id, meta, created_at, updated_at, created_by, updated_by
		FROM auditlog
		WHERE id = ?
	`
	entity, err := scanAuditLog(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created auditlog: %v", err)
	}

	return &pb.CreateAuditLogResponse{
		AuditLog: entity,
	}, nil
}

// UpdateAuditLog applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateAuditLog(ctx context.Context, req *pb.UpdateAuditLogRequest) (*pb.UpdateAuditLogResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildAuditLogWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Meta
	if req.Meta != nil {
		updateFields = append(updateFields, "meta = ?")
		args = append(args, *req.Meta)

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

	query := fmt.Sprintf(`UPDATE auditlog SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update auditlog: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, actor_id, action, target_type, target_id, meta, created_at, updated_at, created_by, updated_by FROM auditlog %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated auditlogs: %v", err)
	}
	defer rows.Close()

	entities := []*pb.AuditLog{}
	for rows.Next() {
		entity, err := scanAuditLog(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan auditlog: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating auditlogs: %v", err)
	}

	return &pb.UpdateAuditLogResponse{
		AuditLog:      entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteAuditLog deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteAuditLog(ctx context.Context, req *pb.DeleteAuditLogRequest) (*pb.DeleteAuditLogResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildAuditLogWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM auditlog %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete auditlog: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteAuditLogResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListAuditLog lists AuditLogs with pagination and filtering
func (h *Handler) ListAuditLog(ctx context.Context, req *pb.ListAuditLogRequest) (*pb.ListAuditLogResponse, error) {
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
		whereClause, args, err = buildAuditLogWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM auditlog %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count auditlogs: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, actor_id, action, target_type, target_id, meta, created_at, updated_at, created_by, updated_by
		FROM auditlog
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list auditlogs: %v", err)
	}
	defer rows.Close()

	entities := []*pb.AuditLog{}
	for rows.Next() {
		entity, err := scanAuditLog(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan auditlog: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating auditlogs: %v", err)
	}

	return &pb.ListAuditLogResponse{
		AuditLog: entities,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
