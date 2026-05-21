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

// scanMaterial reads a single row into a *pb.Material.
// Shared by Create/Update inline-select and the List handler.
func scanMaterial(scanner interface{ Scan(...interface{}) error }) (*pb.Material, error) {
	var entity pb.Material
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var SourceTypeStr string
	var VisibilityStr string
	var CommunityStatusStr string
	var DescriptionNull sql.NullString
	var UrlNull sql.NullString
	var StorageKeyNull sql.NullString
	var FileNameNull sql.NullString
	var FileTypeNull sql.NullString
	var SizeBytesNull sql.NullInt64
	var RejectReasonNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.OwnerTeacherId,
		&entity.Title,
		&DescriptionNull,
		&SourceTypeStr,
		&UrlNull,
		&StorageKeyNull,
		&FileNameNull,
		&FileTypeNull,
		&SizeBytesNull,
		&VisibilityStr,
		&CommunityStatusStr,
		&RejectReasonNull,
		&entity.DownloadCount,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch SourceTypeStr {
	case "material_source_unspecified":
		entity.SourceType = pb.MaterialSourceType_MATERIAL_SOURCE_UNSPECIFIED
	case "source_url":
		entity.SourceType = pb.MaterialSourceType_SOURCE_URL
	case "source_file":
		entity.SourceType = pb.MaterialSourceType_SOURCE_FILE
	default:
		entity.SourceType = pb.MaterialSourceType_MATERIAL_SOURCE_UNSPECIFIED
	}
	switch VisibilityStr {
	case "material_visibility_unspecified":
		entity.Visibility = pb.MaterialVisibility_MATERIAL_VISIBILITY_UNSPECIFIED
	case "mat_class":
		entity.Visibility = pb.MaterialVisibility_MAT_CLASS
	case "mat_community":
		entity.Visibility = pb.MaterialVisibility_MAT_COMMUNITY
	default:
		entity.Visibility = pb.MaterialVisibility_MATERIAL_VISIBILITY_UNSPECIFIED
	}
	// Optional enum CommunityStatus: assign via pointer so unset/NULL stays nil.
	var CommunityStatusVal pb.CommunityStatus
	switch CommunityStatusStr {
	case "community_status_unspecified":
		CommunityStatusVal = pb.CommunityStatus_COMMUNITY_STATUS_UNSPECIFIED
	case "comm_draft":
		CommunityStatusVal = pb.CommunityStatus_COMM_DRAFT
	case "comm_pending":
		CommunityStatusVal = pb.CommunityStatus_COMM_PENDING
	case "comm_approved":
		CommunityStatusVal = pb.CommunityStatus_COMM_APPROVED
	case "comm_rejected":
		CommunityStatusVal = pb.CommunityStatus_COMM_REJECTED
	default:
		CommunityStatusVal = pb.CommunityStatus_COMMUNITY_STATUS_UNSPECIFIED
	}
	entity.CommunityStatus = &CommunityStatusVal

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
	if DescriptionNull.Valid {
		val := DescriptionNull.String
		entity.Description = &val
	}
	if UrlNull.Valid {
		val := UrlNull.String
		entity.Url = &val
	}
	if StorageKeyNull.Valid {
		val := StorageKeyNull.String
		entity.StorageKey = &val
	}
	if FileNameNull.Valid {
		val := FileNameNull.String
		entity.FileName = &val
	}
	if FileTypeNull.Valid {
		val := FileTypeNull.String
		entity.FileType = &val
	}
	if SizeBytesNull.Valid {
		val := SizeBytesNull.Int64
		entity.SizeBytes = &val
	}
	if RejectReasonNull.Valid {
		val := RejectReasonNull.String
		entity.RejectReason = &val
	}

	return &entity, nil
}

// buildMaterialWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in material_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildMaterialWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, MaterialFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, MaterialFilterableFields)
	return clause, args, err
}

// CreateMaterial creates a new Material record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateMaterial(ctx context.Context, req *pb.CreateMaterialRequest) (*pb.CreateMaterialResponse, error) {
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
	// Optional string: Url
	var Url interface{}
	if req.Url != nil {
		Url = *req.Url
	}
	// Optional string: StorageKey
	var StorageKey interface{}
	if req.StorageKey != nil {
		StorageKey = *req.StorageKey
	}
	// Optional string: FileName
	var FileName interface{}
	if req.FileName != nil {
		FileName = *req.FileName
	}
	// Optional string: FileType
	var FileType interface{}
	if req.FileType != nil {
		FileType = *req.FileType
	}
	// Optional int64: SizeBytes
	var SizeBytes interface{}
	if req.SizeBytes != nil {
		SizeBytes = *req.SizeBytes
	}
	// Optional string: RejectReason
	var RejectReason interface{}
	if req.RejectReason != nil {
		RejectReason = *req.RejectReason
	}

	// Convert SourceType enum to string
	SourceTypeValue := pb.MaterialSourceType_MATERIAL_SOURCE_UNSPECIFIED

	SourceTypeValue = req.SourceType
	SourceTypeStr := "material_source_unspecified"
	switch SourceTypeValue {
	case pb.MaterialSourceType_MATERIAL_SOURCE_UNSPECIFIED:
		SourceTypeStr = "material_source_unspecified"
	case pb.MaterialSourceType_SOURCE_URL:
		SourceTypeStr = "source_url"
	case pb.MaterialSourceType_SOURCE_FILE:
		SourceTypeStr = "source_file"
	}
	// Convert Visibility enum to string
	VisibilityValue := pb.MaterialVisibility_MATERIAL_VISIBILITY_UNSPECIFIED

	VisibilityValue = req.Visibility
	VisibilityStr := "material_visibility_unspecified"
	switch VisibilityValue {
	case pb.MaterialVisibility_MATERIAL_VISIBILITY_UNSPECIFIED:
		VisibilityStr = "material_visibility_unspecified"
	case pb.MaterialVisibility_MAT_CLASS:
		VisibilityStr = "mat_class"
	case pb.MaterialVisibility_MAT_COMMUNITY:
		VisibilityStr = "mat_community"
	}
	// Convert CommunityStatus enum to string
	CommunityStatusValue := pb.CommunityStatus_COMMUNITY_STATUS_UNSPECIFIED
	if req.CommunityStatus != nil {
		CommunityStatusValue = *req.CommunityStatus
	}
	CommunityStatusStr := "community_status_unspecified"
	switch CommunityStatusValue {
	case pb.CommunityStatus_COMMUNITY_STATUS_UNSPECIFIED:
		CommunityStatusStr = "community_status_unspecified"
	case pb.CommunityStatus_COMM_DRAFT:
		CommunityStatusStr = "comm_draft"
	case pb.CommunityStatus_COMM_PENDING:
		CommunityStatusStr = "comm_pending"
	case pb.CommunityStatus_COMM_APPROVED:
		CommunityStatusStr = "comm_approved"
	case pb.CommunityStatus_COMM_REJECTED:
		CommunityStatusStr = "comm_rejected"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO material (id, owner_teacher_id, title, description, source_type, url, storage_key, file_name, file_type, size_bytes, visibility, community_status, reject_reason, download_count, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.OwnerTeacherId,
		req.Title,
		Description,
		SourceTypeStr,
		Url,
		StorageKey,
		FileName,
		FileType,
		SizeBytes,
		VisibilityStr,
		CommunityStatusStr,
		RejectReason,
		req.DownloadCount,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "material already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create material: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetMaterial call).
	selectQuery := `
		SELECT id, owner_teacher_id, title, description, source_type, url, storage_key, file_name, file_type, size_bytes, visibility, community_status, reject_reason, download_count, created_at, updated_at, created_by, updated_by
		FROM material
		WHERE id = ?
	`
	entity, err := scanMaterial(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created material: %v", err)
	}

	return &pb.CreateMaterialResponse{
		Material: entity,
	}, nil
}

// UpdateMaterial applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateMaterial(ctx context.Context, req *pb.UpdateMaterialRequest) (*pb.UpdateMaterialResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildMaterialWhere(req.GetFilters())
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
	// Optional field: Url
	if req.Url != nil {
		updateFields = append(updateFields, "url = ?")
		args = append(args, *req.Url)

	}
	// Optional field: Visibility
	if req.Visibility != nil {
		updateFields = append(updateFields, "visibility = ?")
		VisibilityStr := "material_visibility_unspecified"
		switch *req.Visibility {
		case pb.MaterialVisibility_MATERIAL_VISIBILITY_UNSPECIFIED:
			VisibilityStr = "material_visibility_unspecified"
		case pb.MaterialVisibility_MAT_CLASS:
			VisibilityStr = "mat_class"
		case pb.MaterialVisibility_MAT_COMMUNITY:
			VisibilityStr = "mat_community"
		}
		args = append(args, VisibilityStr)

	}
	// Required field: CommunityStatus
	updateFields = append(updateFields, "community_status = ?")
	CommunityStatusStr := "community_status_unspecified"
	switch req.CommunityStatus {
	case pb.CommunityStatus_COMMUNITY_STATUS_UNSPECIFIED:
		CommunityStatusStr = "community_status_unspecified"
	case pb.CommunityStatus_COMM_DRAFT:
		CommunityStatusStr = "comm_draft"
	case pb.CommunityStatus_COMM_PENDING:
		CommunityStatusStr = "comm_pending"
	case pb.CommunityStatus_COMM_APPROVED:
		CommunityStatusStr = "comm_approved"
	case pb.CommunityStatus_COMM_REJECTED:
		CommunityStatusStr = "comm_rejected"
	}
	args = append(args, CommunityStatusStr)

	// Optional field: RejectReason
	if req.RejectReason != nil {
		updateFields = append(updateFields, "reject_reason = ?")
		args = append(args, *req.RejectReason)

	}
	// Optional field: DownloadCount
	if req.DownloadCount != nil {
		updateFields = append(updateFields, "download_count = ?")
		args = append(args, *req.DownloadCount)

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

	query := fmt.Sprintf(`UPDATE material SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update material: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, owner_teacher_id, title, description, source_type, url, storage_key, file_name, file_type, size_bytes, visibility, community_status, reject_reason, download_count, created_at, updated_at, created_by, updated_by FROM material %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated materials: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Material{}
	for rows.Next() {
		entity, err := scanMaterial(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan material: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating materials: %v", err)
	}

	return &pb.UpdateMaterialResponse{
		Material:      entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteMaterial deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteMaterial(ctx context.Context, req *pb.DeleteMaterialRequest) (*pb.DeleteMaterialResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildMaterialWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM material %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete material: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteMaterialResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListMaterial lists Materials with pagination and filtering
func (h *Handler) ListMaterial(ctx context.Context, req *pb.ListMaterialRequest) (*pb.ListMaterialResponse, error) {
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
		whereClause, args, err = buildMaterialWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM material %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count materials: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, owner_teacher_id, title, description, source_type, url, storage_key, file_name, file_type, size_bytes, visibility, community_status, reject_reason, download_count, created_at, updated_at, created_by, updated_by
		FROM material
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list materials: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Material{}
	for rows.Next() {
		entity, err := scanMaterial(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan material: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating materials: %v", err)
	}

	return &pb.ListMaterialResponse{
		Material: entities,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
