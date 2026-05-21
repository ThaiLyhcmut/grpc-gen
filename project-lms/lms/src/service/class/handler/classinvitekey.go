package handler

import (
	"context"
	"database/sql"
	"fmt"
	pb "github.com/thaily/lms/proto/class"
	commonpb "github.com/thaily/lms/proto/common"
	"github.com/thaily/lms/src/service/pkg/helper"
	"github.com/thaily/lms/src/service/pkg/logger"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// scanClassInviteKey reads a single row into a *pb.ClassInviteKey.
// Shared by Create/Update inline-select and the List handler.
func scanClassInviteKey(scanner interface{ Scan(...interface{}) error }) (*pb.ClassInviteKey, error) {
	var entity pb.ClassInviteKey
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var StatusStr string
	var ExpiresAtTime sql.NullTime
	var UsedAtTime sql.NullTime
	var NoteNull sql.NullString
	var UsedByStudentIdNull sql.NullInt64
	var TargetStudentIdNull sql.NullInt64

	err := scanner.Scan(
		&entity.Id,
		&entity.ClassId,
		&entity.KeyCode,
		&entity.CreatedByTeacherId,
		&NoteNull,
		&ExpiresAtTime,
		&UsedAtTime,
		&UsedByStudentIdNull,
		&TargetStudentIdNull,
		&StatusStr,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch StatusStr {
	case "invite_key_status_unspecified":
		entity.Status = pb.InviteKeyStatus_INVITE_KEY_STATUS_UNSPECIFIED
	case "key_active":
		entity.Status = pb.InviteKeyStatus_KEY_ACTIVE
	case "key_used":
		entity.Status = pb.InviteKeyStatus_KEY_USED
	case "key_revoked":
		entity.Status = pb.InviteKeyStatus_KEY_REVOKED
	case "key_expired":
		entity.Status = pb.InviteKeyStatus_KEY_EXPIRED
	default:
		entity.Status = pb.InviteKeyStatus_INVITE_KEY_STATUS_UNSPECIFIED
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
	if NoteNull.Valid {
		val := NoteNull.String
		entity.Note = &val
	}
	if UsedByStudentIdNull.Valid {
		val := uint64(UsedByStudentIdNull.Int64)
		entity.UsedByStudentId = &val
	}
	if TargetStudentIdNull.Valid {
		val := uint64(TargetStudentIdNull.Int64)
		entity.TargetStudentId = &val
	}

	return &entity, nil
}

// buildClassInviteKeyWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in classinvitekey_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildClassInviteKeyWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, ClassInviteKeyFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, ClassInviteKeyFilterableFields)
	return clause, args, err
}

// CreateClassInviteKey creates a new ClassInviteKey record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateClassInviteKey(ctx context.Context, req *pb.CreateClassInviteKeyRequest) (*pb.CreateClassInviteKeyResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.KeyCode == "" {
		return nil, status.Error(codes.InvalidArgument, "key_code is required")
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
	// Optional string: Note
	var Note interface{}
	if req.Note != nil {
		Note = *req.Note
	}
	// Optional timestamp: ExpiresAt
	var ExpiresAt interface{}
	if req.ExpiresAt != nil {
		ExpiresAt = req.ExpiresAt.AsTime()
	}
	// Optional timestamp: UsedAt
	var UsedAt interface{}
	if req.UsedAt != nil {
		UsedAt = req.UsedAt.AsTime()
	}
	// Optional uint64: UsedByStudentId
	var UsedByStudentId interface{}
	if req.UsedByStudentId != nil {
		UsedByStudentId = *req.UsedByStudentId
	}
	// Optional uint64: TargetStudentId
	var TargetStudentId interface{}
	if req.TargetStudentId != nil {
		TargetStudentId = *req.TargetStudentId
	}

	// Convert Status enum to string
	StatusValue := pb.InviteKeyStatus_INVITE_KEY_STATUS_UNSPECIFIED

	StatusValue = req.Status
	StatusStr := "invite_key_status_unspecified"
	switch StatusValue {
	case pb.InviteKeyStatus_INVITE_KEY_STATUS_UNSPECIFIED:
		StatusStr = "invite_key_status_unspecified"
	case pb.InviteKeyStatus_KEY_ACTIVE:
		StatusStr = "key_active"
	case pb.InviteKeyStatus_KEY_USED:
		StatusStr = "key_used"
	case pb.InviteKeyStatus_KEY_REVOKED:
		StatusStr = "key_revoked"
	case pb.InviteKeyStatus_KEY_EXPIRED:
		StatusStr = "key_expired"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO classinvitekey (id, class_id, key_code, created_by_teacher_id, note, expires_at, used_at, used_by_student_id, target_student_id, status, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.ClassId,
		req.KeyCode,
		req.CreatedByTeacherId,
		Note,
		ExpiresAt,
		UsedAt,
		UsedByStudentId,
		TargetStudentId,
		StatusStr,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "classinvitekey already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create classinvitekey: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetClassInviteKey call).
	selectQuery := `
		SELECT id, class_id, key_code, created_by_teacher_id, note, expires_at, used_at, used_by_student_id, target_student_id, status, created_at, updated_at, created_by, updated_by
		FROM classinvitekey
		WHERE id = ?
	`
	entity, err := scanClassInviteKey(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created classinvitekey: %v", err)
	}

	return &pb.CreateClassInviteKeyResponse{
		ClassInviteKey: entity,
	}, nil
}

// UpdateClassInviteKey applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateClassInviteKey(ctx context.Context, req *pb.UpdateClassInviteKeyRequest) (*pb.UpdateClassInviteKeyResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildClassInviteKeyWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Note
	if req.Note != nil {
		updateFields = append(updateFields, "note = ?")
		args = append(args, *req.Note)

	}
	// Optional field: ExpiresAt
	if req.ExpiresAt != nil {
		updateFields = append(updateFields, "expires_at = ?")
		args = append(args, req.ExpiresAt.AsTime())

	}
	// Optional field: UsedAt
	if req.UsedAt != nil {
		updateFields = append(updateFields, "used_at = ?")
		args = append(args, req.UsedAt.AsTime())

	}
	// Optional field: UsedByStudentId
	if req.UsedByStudentId != nil {
		updateFields = append(updateFields, "used_by_student_id = ?")
		args = append(args, *req.UsedByStudentId)

	}
	// Optional field: Status
	if req.Status != nil {
		updateFields = append(updateFields, "status = ?")
		StatusStr := "invite_key_status_unspecified"
		switch *req.Status {
		case pb.InviteKeyStatus_INVITE_KEY_STATUS_UNSPECIFIED:
			StatusStr = "invite_key_status_unspecified"
		case pb.InviteKeyStatus_KEY_ACTIVE:
			StatusStr = "key_active"
		case pb.InviteKeyStatus_KEY_USED:
			StatusStr = "key_used"
		case pb.InviteKeyStatus_KEY_REVOKED:
			StatusStr = "key_revoked"
		case pb.InviteKeyStatus_KEY_EXPIRED:
			StatusStr = "key_expired"
		}
		args = append(args, StatusStr)

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

	query := fmt.Sprintf(`UPDATE classinvitekey SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update classinvitekey: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, class_id, key_code, created_by_teacher_id, note, expires_at, used_at, used_by_student_id, target_student_id, status, created_at, updated_at, created_by, updated_by FROM classinvitekey %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated classinvitekeys: %v", err)
	}
	defer rows.Close()

	entities := []*pb.ClassInviteKey{}
	for rows.Next() {
		entity, err := scanClassInviteKey(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan classinvitekey: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating classinvitekeys: %v", err)
	}

	return &pb.UpdateClassInviteKeyResponse{
		ClassInviteKey: entities,
		AffectedCount:  int32(affected),
	}, nil
}

// DeleteClassInviteKey deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteClassInviteKey(ctx context.Context, req *pb.DeleteClassInviteKeyRequest) (*pb.DeleteClassInviteKeyResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildClassInviteKeyWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM classinvitekey %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete classinvitekey: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteClassInviteKeyResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListClassInviteKey lists ClassInviteKeys with pagination and filtering
func (h *Handler) ListClassInviteKey(ctx context.Context, req *pb.ListClassInviteKeyRequest) (*pb.ListClassInviteKeyResponse, error) {
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
		whereClause, args, err = buildClassInviteKeyWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM classinvitekey %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count classinvitekeys: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, class_id, key_code, created_by_teacher_id, note, expires_at, used_at, used_by_student_id, target_student_id, status, created_at, updated_at, created_by, updated_by
		FROM classinvitekey
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list classinvitekeys: %v", err)
	}
	defer rows.Close()

	entities := []*pb.ClassInviteKey{}
	for rows.Next() {
		entity, err := scanClassInviteKey(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan classinvitekey: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating classinvitekeys: %v", err)
	}

	return &pb.ListClassInviteKeyResponse{
		ClassInviteKey: entities,
		Total:          total,
		Page:           page,
		PageSize:       pageSize,
	}, nil
}
