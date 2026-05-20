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

// scanUser reads a single row into a *pb.User.
// Shared by Create/Update inline-select and the List handler.
func scanUser(scanner interface{ Scan(...interface{}) error }) (*pb.User, error) {
	var entity pb.User
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var RoleStr string
	var StatusStr string
	var EmailVerifiedAtTime sql.NullTime
	var LastLoginAtTime sql.NullTime
	var PhoneNull sql.NullString
	var AvatarUrlNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.Email,
		&PhoneNull,
		&entity.PasswordHash,
		&entity.FullName,
		&AvatarUrlNull,
		&RoleStr,
		&StatusStr,
		&EmailVerifiedAtTime,
		&LastLoginAtTime,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch RoleStr {
	case "user_role_unspecified":
		entity.Role = pb.UserRole_USER_ROLE_UNSPECIFIED
	case "admin":
		entity.Role = pb.UserRole_ADMIN
	case "teacher":
		entity.Role = pb.UserRole_TEACHER
	case "student":
		entity.Role = pb.UserRole_STUDENT
	default:
		entity.Role = pb.UserRole_USER_ROLE_UNSPECIFIED
	}
	switch StatusStr {
	case "user_status_unspecified":
		entity.Status = pb.UserStatus_USER_STATUS_UNSPECIFIED
	case "active":
		entity.Status = pb.UserStatus_ACTIVE
	case "banned":
		entity.Status = pb.UserStatus_BANNED
	case "pending_verify":
		entity.Status = pb.UserStatus_PENDING_VERIFY
	default:
		entity.Status = pb.UserStatus_USER_STATUS_UNSPECIFIED
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
	if EmailVerifiedAtTime.Valid {
		entity.EmailVerifiedAt = timestamppb.New(EmailVerifiedAtTime.Time)
	}
	if LastLoginAtTime.Valid {
		entity.LastLoginAt = timestamppb.New(LastLoginAtTime.Time)
	}
	if PhoneNull.Valid {
		val := PhoneNull.String
		entity.Phone = &val
	}
	if AvatarUrlNull.Valid {
		val := AvatarUrlNull.String
		entity.AvatarUrl = &val
	}

	return &entity, nil
}

// buildUserWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in user_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildUserWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, UserFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, UserFilterableFields)
	return clause, args, err
}

// CreateUser creates a new User record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	if req.PasswordHash == "" {
		return nil, status.Error(codes.InvalidArgument, "password_hash is required")
	}
	if req.FullName == "" {
		return nil, status.Error(codes.InvalidArgument, "full_name is required")
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
	// Optional string: Phone
	var Phone interface{}
	if req.Phone != nil {
		Phone = *req.Phone
	}
	// Optional string: AvatarUrl
	var AvatarUrl interface{}
	if req.AvatarUrl != nil {
		AvatarUrl = *req.AvatarUrl
	}
	// Optional timestamp: EmailVerifiedAt
	var EmailVerifiedAt interface{}
	if req.EmailVerifiedAt != nil {
		EmailVerifiedAt = req.EmailVerifiedAt.AsTime()
	}
	// Optional timestamp: LastLoginAt
	var LastLoginAt interface{}
	if req.LastLoginAt != nil {
		LastLoginAt = req.LastLoginAt.AsTime()
	}

	// Convert Role enum to string
	RoleValue := pb.UserRole_USER_ROLE_UNSPECIFIED

	RoleValue = req.Role
	RoleStr := "user_role_unspecified"
	switch RoleValue {
	case pb.UserRole_USER_ROLE_UNSPECIFIED:
		RoleStr = "user_role_unspecified"
	case pb.UserRole_ADMIN:
		RoleStr = "admin"
	case pb.UserRole_TEACHER:
		RoleStr = "teacher"
	case pb.UserRole_STUDENT:
		RoleStr = "student"
	}
	// Convert Status enum to string
	StatusValue := pb.UserStatus_USER_STATUS_UNSPECIFIED

	StatusValue = req.Status
	StatusStr := "user_status_unspecified"
	switch StatusValue {
	case pb.UserStatus_USER_STATUS_UNSPECIFIED:
		StatusStr = "user_status_unspecified"
	case pb.UserStatus_ACTIVE:
		StatusStr = "active"
	case pb.UserStatus_BANNED:
		StatusStr = "banned"
	case pb.UserStatus_PENDING_VERIFY:
		StatusStr = "pending_verify"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO user (id, email, phone, password_hash, full_name, avatar_url, role, status, email_verified_at, last_login_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.Email,
		Phone,
		req.PasswordHash,
		req.FullName,
		AvatarUrl,
		RoleStr,
		StatusStr,
		EmailVerifiedAt,
		LastLoginAt,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "user already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create user: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetUser call).
	selectQuery := `
		SELECT id, email, phone, password_hash, full_name, avatar_url, role, status, email_verified_at, last_login_at, created_at, updated_at, created_by, updated_by
		FROM user
		WHERE id = ?
	`
	entity, err := scanUser(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created user: %v", err)
	}

	return &pb.CreateUserResponse{
		User: entity,
	}, nil
}

// UpdateUser applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UpdateUserResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildUserWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Email
	if req.Email != nil {
		updateFields = append(updateFields, "email = ?")
		args = append(args, *req.Email)

	}
	// Optional field: Phone
	if req.Phone != nil {
		updateFields = append(updateFields, "phone = ?")
		args = append(args, *req.Phone)

	}
	// Optional field: PasswordHash
	if req.PasswordHash != nil {
		updateFields = append(updateFields, "password_hash = ?")
		args = append(args, *req.PasswordHash)

	}
	// Optional field: FullName
	if req.FullName != nil {
		updateFields = append(updateFields, "full_name = ?")
		args = append(args, *req.FullName)

	}
	// Optional field: AvatarUrl
	if req.AvatarUrl != nil {
		updateFields = append(updateFields, "avatar_url = ?")
		args = append(args, *req.AvatarUrl)

	}
	// Optional field: Role
	if req.Role != nil {
		updateFields = append(updateFields, "role = ?")
		RoleStr := "user_role_unspecified"
		switch *req.Role {
		case pb.UserRole_USER_ROLE_UNSPECIFIED:
			RoleStr = "user_role_unspecified"
		case pb.UserRole_ADMIN:
			RoleStr = "admin"
		case pb.UserRole_TEACHER:
			RoleStr = "teacher"
		case pb.UserRole_STUDENT:
			RoleStr = "student"
		}
		args = append(args, RoleStr)

	}
	// Optional field: Status
	if req.Status != nil {
		updateFields = append(updateFields, "status = ?")
		StatusStr := "user_status_unspecified"
		switch *req.Status {
		case pb.UserStatus_USER_STATUS_UNSPECIFIED:
			StatusStr = "user_status_unspecified"
		case pb.UserStatus_ACTIVE:
			StatusStr = "active"
		case pb.UserStatus_BANNED:
			StatusStr = "banned"
		case pb.UserStatus_PENDING_VERIFY:
			StatusStr = "pending_verify"
		}
		args = append(args, StatusStr)

	}
	// Optional field: EmailVerifiedAt
	if req.EmailVerifiedAt != nil {
		updateFields = append(updateFields, "email_verified_at = ?")
		args = append(args, req.EmailVerifiedAt.AsTime())

	}
	// Optional field: LastLoginAt
	if req.LastLoginAt != nil {
		updateFields = append(updateFields, "last_login_at = ?")
		args = append(args, req.LastLoginAt.AsTime())

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

	query := fmt.Sprintf(`UPDATE user SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update user: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, email, phone, password_hash, full_name, avatar_url, role, status, email_verified_at, last_login_at, created_at, updated_at, created_by, updated_by FROM user %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated users: %v", err)
	}
	defer rows.Close()

	entities := []*pb.User{}
	for rows.Next() {
		entity, err := scanUser(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan user: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating users: %v", err)
	}

	return &pb.UpdateUserResponse{
		User:          entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteUser deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildUserWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM user %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete user: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteUserResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListUser lists Users with pagination and filtering
func (h *Handler) ListUser(ctx context.Context, req *pb.ListUserRequest) (*pb.ListUserResponse, error) {
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
		whereClause, args, err = buildUserWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM user %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count users: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, email, phone, password_hash, full_name, avatar_url, role, status, email_verified_at, last_login_at, created_at, updated_at, created_by, updated_by
		FROM user
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list users: %v", err)
	}
	defer rows.Close()

	entities := []*pb.User{}
	for rows.Next() {
		entity, err := scanUser(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan user: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating users: %v", err)
	}

	return &pb.ListUserResponse{
		User:     entities,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
