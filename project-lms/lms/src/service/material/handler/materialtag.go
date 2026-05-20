package handler

import (
	"context"
	"database/sql"
	"fmt"
	commonpb "github.com/thaily/lms/proto/common"
	pb "github.com/thaily/lms/proto/material"
	"github.com/thaily/lms/src/service/pkg/helper"
	"github.com/thaily/lms/src/service/pkg/logger"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// scanMaterialTag reads a single row into a *pb.MaterialTag.
// Shared by Create/Update inline-select and the List handler.
func scanMaterialTag(scanner interface{ Scan(...interface{}) error }) (*pb.MaterialTag, error) {
	var entity pb.MaterialTag
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.MaterialId,
		&entity.Tag,
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

// buildMaterialTagWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in materialtag_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildMaterialTagWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, MaterialTagFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, MaterialTagFilterableFields)
	return clause, args, err
}

// CreateMaterialTag creates a new MaterialTag record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateMaterialTag(ctx context.Context, req *pb.CreateMaterialTagRequest) (*pb.CreateMaterialTagResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.Tag == "" {
		return nil, status.Error(codes.InvalidArgument, "tag is required")
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
		INSERT INTO materialtag (id, material_id, tag, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.MaterialId,
		req.Tag,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "materialtag already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create materialtag: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetMaterialTag call).
	selectQuery := `
		SELECT id, material_id, tag, created_at, updated_at, created_by, updated_by
		FROM materialtag
		WHERE id = ?
	`
	entity, err := scanMaterialTag(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created materialtag: %v", err)
	}

	return &pb.CreateMaterialTagResponse{
		MaterialTag: entity,
	}, nil
}

// UpdateMaterialTag applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateMaterialTag(ctx context.Context, req *pb.UpdateMaterialTagRequest) (*pb.UpdateMaterialTagResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildMaterialTagWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Tag
	if req.Tag != nil {
		updateFields = append(updateFields, "tag = ?")
		args = append(args, *req.Tag)

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

	query := fmt.Sprintf(`UPDATE materialtag SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update materialtag: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, material_id, tag, created_at, updated_at, created_by, updated_by FROM materialtag %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated materialtags: %v", err)
	}
	defer rows.Close()

	entities := []*pb.MaterialTag{}
	for rows.Next() {
		entity, err := scanMaterialTag(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan materialtag: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating materialtags: %v", err)
	}

	return &pb.UpdateMaterialTagResponse{
		MaterialTag:   entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteMaterialTag deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteMaterialTag(ctx context.Context, req *pb.DeleteMaterialTagRequest) (*pb.DeleteMaterialTagResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildMaterialTagWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM materialtag %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete materialtag: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteMaterialTagResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListMaterialTag lists MaterialTags with pagination and filtering
func (h *Handler) ListMaterialTag(ctx context.Context, req *pb.ListMaterialTagRequest) (*pb.ListMaterialTagResponse, error) {
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
		whereClause, args, err = buildMaterialTagWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM materialtag %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count materialtags: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, material_id, tag, created_at, updated_at, created_by, updated_by
		FROM materialtag
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list materialtags: %v", err)
	}
	defer rows.Close()

	entities := []*pb.MaterialTag{}
	for rows.Next() {
		entity, err := scanMaterialTag(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan materialtag: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating materialtags: %v", err)
	}

	return &pb.ListMaterialTagResponse{
		MaterialTag: entities,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}
