package handler

import (
	"context"
	"database/sql"
	"fmt"
	commonpb "github.com/thaily/lms/proto/common"
	pb "github.com/thaily/lms/proto/notification"
	"github.com/thaily/lms/src/service/pkg/helper"
	"github.com/thaily/lms/src/service/pkg/logger"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// scanNotification reads a single row into a *pb.Notification.
// Shared by Create/Update inline-select and the List handler.
func scanNotification(scanner interface{ Scan(...interface{}) error }) (*pb.Notification, error) {
	var entity pb.Notification
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var ReadAtTime sql.NullTime
	var BodyNull sql.NullString
	var PayloadNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.UserId,
		&entity.Type,
		&entity.Title,
		&BodyNull,
		&PayloadNull,
		&ReadAtTime,
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
	if ReadAtTime.Valid {
		entity.ReadAt = timestamppb.New(ReadAtTime.Time)
	}
	if BodyNull.Valid {
		val := BodyNull.String
		entity.Body = &val
	}
	if PayloadNull.Valid {
		val := PayloadNull.String
		entity.Payload = &val
	}

	return &entity, nil
}

// buildNotificationWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in notification_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildNotificationWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, NotificationFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, NotificationFilterableFields)
	return clause, args, err
}

// CreateNotification creates a new Notification record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateNotification(ctx context.Context, req *pb.CreateNotificationRequest) (*pb.CreateNotificationResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.Type == "" {
		return nil, status.Error(codes.InvalidArgument, "type is required")
	}
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
	// Optional string: Body
	var Body interface{}
	if req.Body != nil {
		Body = *req.Body
	}
	// Optional string: Payload
	var Payload interface{}
	if req.Payload != nil {
		Payload = *req.Payload
	}
	// Optional timestamp: ReadAt
	var ReadAt interface{}
	if req.ReadAt != nil {
		ReadAt = req.ReadAt.AsTime()
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO notification (id, user_id, type, title, body, payload, read_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.UserId,
		req.Type,
		req.Title,
		Body,
		Payload,
		ReadAt,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "notification already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create notification: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetNotification call).
	selectQuery := `
		SELECT id, user_id, type, title, body, payload, read_at, created_at, updated_at, created_by, updated_by
		FROM notification
		WHERE id = ?
	`
	entity, err := scanNotification(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created notification: %v", err)
	}

	return &pb.CreateNotificationResponse{
		Notification: entity,
	}, nil
}

// UpdateNotification applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateNotification(ctx context.Context, req *pb.UpdateNotificationRequest) (*pb.UpdateNotificationResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildNotificationWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: ReadAt
	if req.ReadAt != nil {
		updateFields = append(updateFields, "read_at = ?")
		args = append(args, req.ReadAt.AsTime())

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

	query := fmt.Sprintf(`UPDATE notification SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update notification: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, user_id, type, title, body, payload, read_at, created_at, updated_at, created_by, updated_by FROM notification %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated notifications: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Notification{}
	for rows.Next() {
		entity, err := scanNotification(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan notification: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating notifications: %v", err)
	}

	return &pb.UpdateNotificationResponse{
		Notification:  entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteNotification deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteNotification(ctx context.Context, req *pb.DeleteNotificationRequest) (*pb.DeleteNotificationResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildNotificationWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM notification %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete notification: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteNotificationResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListNotification lists Notifications with pagination and filtering
func (h *Handler) ListNotification(ctx context.Context, req *pb.ListNotificationRequest) (*pb.ListNotificationResponse, error) {
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
		whereClause, args, err = buildNotificationWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM notification %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count notifications: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, user_id, type, title, body, payload, read_at, created_at, updated_at, created_by, updated_by
		FROM notification
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list notifications: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Notification{}
	for rows.Next() {
		entity, err := scanNotification(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan notification: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating notifications: %v", err)
	}

	return &pb.ListNotificationResponse{
		Notification: entities,
		Total:        total,
		Page:         page,
		PageSize:     pageSize,
	}, nil
}
