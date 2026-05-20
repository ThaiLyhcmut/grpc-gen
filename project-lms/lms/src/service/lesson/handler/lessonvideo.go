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

// scanLessonVideo reads a single row into a *pb.LessonVideo.
// Shared by Create/Update inline-select and the List handler.
func scanLessonVideo(scanner interface{ Scan(...interface{}) error }) (*pb.LessonVideo, error) {
	var entity pb.LessonVideo
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var StatusStr string
	var UploadedAtTime sql.NullTime
	var DurationSecNull sql.NullInt32
	var SizeBytesNull sql.NullInt64
	var MimeTypeNull sql.NullString
	var ThumbnailKeyNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.LessonId,
		&entity.Title,
		&entity.StorageKey,
		&DurationSecNull,
		&SizeBytesNull,
		&MimeTypeNull,
		&ThumbnailKeyNull,
		&StatusStr,
		&UploadedAtTime,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch StatusStr {
	case "video_status_unspecified":
		entity.Status = pb.VideoStatus_VIDEO_STATUS_UNSPECIFIED
	case "uploading":
		entity.Status = pb.VideoStatus_UPLOADING
	case "processing":
		entity.Status = pb.VideoStatus_PROCESSING
	case "ready":
		entity.Status = pb.VideoStatus_READY
	case "failed":
		entity.Status = pb.VideoStatus_FAILED
	default:
		entity.Status = pb.VideoStatus_VIDEO_STATUS_UNSPECIFIED
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
	if UploadedAtTime.Valid {
		entity.UploadedAt = timestamppb.New(UploadedAtTime.Time)
	}
	if DurationSecNull.Valid {
		val := DurationSecNull.Int32
		entity.DurationSec = &val
	}
	if SizeBytesNull.Valid {
		val := SizeBytesNull.Int64
		entity.SizeBytes = &val
	}
	if MimeTypeNull.Valid {
		val := MimeTypeNull.String
		entity.MimeType = &val
	}
	if ThumbnailKeyNull.Valid {
		val := ThumbnailKeyNull.String
		entity.ThumbnailKey = &val
	}

	return &entity, nil
}

// buildLessonVideoWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in lessonvideo_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildLessonVideoWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, LessonVideoFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, LessonVideoFilterableFields)
	return clause, args, err
}

// CreateLessonVideo creates a new LessonVideo record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateLessonVideo(ctx context.Context, req *pb.CreateLessonVideoRequest) (*pb.CreateLessonVideoResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}
	if req.StorageKey == "" {
		return nil, status.Error(codes.InvalidArgument, "storage_key is required")
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
	// Optional int32: DurationSec
	var DurationSec interface{}
	if req.DurationSec != nil {
		DurationSec = *req.DurationSec
	}
	// Optional int64: SizeBytes
	var SizeBytes interface{}
	if req.SizeBytes != nil {
		SizeBytes = *req.SizeBytes
	}
	// Optional string: MimeType
	var MimeType interface{}
	if req.MimeType != nil {
		MimeType = *req.MimeType
	}
	// Optional string: ThumbnailKey
	var ThumbnailKey interface{}
	if req.ThumbnailKey != nil {
		ThumbnailKey = *req.ThumbnailKey
	}
	// Optional timestamp: UploadedAt
	var UploadedAt interface{}
	if req.UploadedAt != nil {
		UploadedAt = req.UploadedAt.AsTime()
	}

	// Convert Status enum to string
	StatusValue := pb.VideoStatus_VIDEO_STATUS_UNSPECIFIED

	StatusValue = req.Status
	StatusStr := "video_status_unspecified"
	switch StatusValue {
	case pb.VideoStatus_VIDEO_STATUS_UNSPECIFIED:
		StatusStr = "video_status_unspecified"
	case pb.VideoStatus_UPLOADING:
		StatusStr = "uploading"
	case pb.VideoStatus_PROCESSING:
		StatusStr = "processing"
	case pb.VideoStatus_READY:
		StatusStr = "ready"
	case pb.VideoStatus_FAILED:
		StatusStr = "failed"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO lessonvideo (id, lesson_id, title, storage_key, duration_sec, size_bytes, mime_type, thumbnail_key, status, uploaded_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.LessonId,
		req.Title,
		req.StorageKey,
		DurationSec,
		SizeBytes,
		MimeType,
		ThumbnailKey,
		StatusStr,
		UploadedAt,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "lessonvideo already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create lessonvideo: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetLessonVideo call).
	selectQuery := `
		SELECT id, lesson_id, title, storage_key, duration_sec, size_bytes, mime_type, thumbnail_key, status, uploaded_at, created_at, updated_at, created_by, updated_by
		FROM lessonvideo
		WHERE id = ?
	`
	entity, err := scanLessonVideo(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created lessonvideo: %v", err)
	}

	return &pb.CreateLessonVideoResponse{
		LessonVideo: entity,
	}, nil
}

// UpdateLessonVideo applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateLessonVideo(ctx context.Context, req *pb.UpdateLessonVideoRequest) (*pb.UpdateLessonVideoResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildLessonVideoWhere(req.GetFilters())
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
	// Optional field: DurationSec
	if req.DurationSec != nil {
		updateFields = append(updateFields, "duration_sec = ?")
		args = append(args, *req.DurationSec)

	}
	// Optional field: SizeBytes
	if req.SizeBytes != nil {
		updateFields = append(updateFields, "size_bytes = ?")
		args = append(args, *req.SizeBytes)

	}
	// Optional field: MimeType
	if req.MimeType != nil {
		updateFields = append(updateFields, "mime_type = ?")
		args = append(args, *req.MimeType)

	}
	// Optional field: ThumbnailKey
	if req.ThumbnailKey != nil {
		updateFields = append(updateFields, "thumbnail_key = ?")
		args = append(args, *req.ThumbnailKey)

	}
	// Optional field: Status
	if req.Status != nil {
		updateFields = append(updateFields, "status = ?")
		StatusStr := "video_status_unspecified"
		switch *req.Status {
		case pb.VideoStatus_VIDEO_STATUS_UNSPECIFIED:
			StatusStr = "video_status_unspecified"
		case pb.VideoStatus_UPLOADING:
			StatusStr = "uploading"
		case pb.VideoStatus_PROCESSING:
			StatusStr = "processing"
		case pb.VideoStatus_READY:
			StatusStr = "ready"
		case pb.VideoStatus_FAILED:
			StatusStr = "failed"
		}
		args = append(args, StatusStr)

	}
	// Optional field: UploadedAt
	if req.UploadedAt != nil {
		updateFields = append(updateFields, "uploaded_at = ?")
		args = append(args, req.UploadedAt.AsTime())

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

	query := fmt.Sprintf(`UPDATE lessonvideo SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update lessonvideo: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, lesson_id, title, storage_key, duration_sec, size_bytes, mime_type, thumbnail_key, status, uploaded_at, created_at, updated_at, created_by, updated_by FROM lessonvideo %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated lessonvideos: %v", err)
	}
	defer rows.Close()

	entities := []*pb.LessonVideo{}
	for rows.Next() {
		entity, err := scanLessonVideo(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan lessonvideo: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating lessonvideos: %v", err)
	}

	return &pb.UpdateLessonVideoResponse{
		LessonVideo:   entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteLessonVideo deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteLessonVideo(ctx context.Context, req *pb.DeleteLessonVideoRequest) (*pb.DeleteLessonVideoResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildLessonVideoWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM lessonvideo %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete lessonvideo: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteLessonVideoResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListLessonVideo lists LessonVideos with pagination and filtering
func (h *Handler) ListLessonVideo(ctx context.Context, req *pb.ListLessonVideoRequest) (*pb.ListLessonVideoResponse, error) {
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
		whereClause, args, err = buildLessonVideoWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM lessonvideo %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count lessonvideos: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, lesson_id, title, storage_key, duration_sec, size_bytes, mime_type, thumbnail_key, status, uploaded_at, created_at, updated_at, created_by, updated_by
		FROM lessonvideo
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list lessonvideos: %v", err)
	}
	defer rows.Close()

	entities := []*pb.LessonVideo{}
	for rows.Next() {
		entity, err := scanLessonVideo(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan lessonvideo: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating lessonvideos: %v", err)
	}

	return &pb.ListLessonVideoResponse{
		LessonVideo: entities,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}
