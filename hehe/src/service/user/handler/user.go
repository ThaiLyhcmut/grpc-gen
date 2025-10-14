package handler

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	pb "hehe/proto/user"
	"hehe/src/pkg/helper"
	"hehe/src/pkg/logger"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)



// CreateUser creates a new User record
func (h *Handler) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	
	// Generate UUID
	id := uuid.New().String()

	// Prepare fields
	
	// Convert Status enum to string
	StatusValue := pb.UserStatus_ACTIVE
	
	StatusValue = req.Status
	StatusStr := "active"
	switch StatusValue {
	case pb.UserStatus_ACTIVE:
		StatusStr = "active"
	case pb.UserStatus_INACTIVE:
		StatusStr = "inactive"
	}
	
	// Handle optional created_by field
	var createdBy interface{}
	if req.CreatedBy != nil {
		createdBy = *req.CreatedBy
	} else {
		createdBy = nil
	}

	// Insert into database
	query := `
		INSERT INTO User (id, name, status, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, NOW(), NOW())
	`

	_, err := h.execQuery(ctx, query,
		id,
		req.Name,
		StatusStr,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "user already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create user: %v", err)
	}

	result, err := h.GetUser(ctx, &pb.GetUserRequest{Id: id})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get user")
	}
	return &pb.CreateUserResponse{
		User: result.GetUser(),
	}, nil
}













// GetUser retrieves a User by ID
func (h *Handler) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	defer logger.TraceFunction(ctx)()

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	query := `
		SELECT id, name, status, created_at, updated_at, created_by, updated_by
		FROM User
		WHERE id = ?
	`

	var entity pb.User
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var StatusStr string
	
	err := h.queryRow(ctx, query, req.Id).Scan(
		&entity.Id,
		&entity.Name,
		&StatusStr,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}

	// Convert Status string to enum
	switch StatusStr {
	case "active":
		entity.Status = pb.UserStatus_ACTIVE
	case "inactive":
		entity.Status = pb.UserStatus_INACTIVE
	default:
		entity.Status = pb.UserStatus_ACTIVE
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

	return &pb.GetUserResponse{
		User: &entity,
	}, nil
}













// UpdateUser updates an existing User
func (h *Handler) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UpdateUserResponse, error) {
	defer logger.TraceFunction(ctx)()

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	// Build dynamic update query
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Name
	if req.Name != nil {
		updateFields = append(updateFields, "name = ?")
		args = append(args, *req.Name)
		
	}
	// Required field: Status
	updateFields = append(updateFields, "status = ?")
	StatusStr := "active"
	switch req.Status {
	case pb.UserStatus_ACTIVE:
		StatusStr = "active"
	case pb.UserStatus_INACTIVE:
		StatusStr = "inactive"
	}
	args = append(args, StatusStr)
	
	
	if len(updateFields) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no fields to update")
	}

	// Add updated_by and updated_at
	updateFields = append(updateFields, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	updateFields = append(updateFields, "updated_at = NOW()")

	// Add id as last parameter
	args = append(args, req.Id)

	query := fmt.Sprintf(`
		UPDATE User
		SET %s
		WHERE id = ?
	`, strings.Join(updateFields, ", "))

	_, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update user: %v", err)
	}

	result, err := h.GetUser(ctx, &pb.GetUserRequest{Id: req.Id})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get user")
	}
	return &pb.UpdateUserResponse{
		User: result.GetUser(),
	}, nil
}













// DeleteUser deletes a User by ID
func (h *Handler) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	defer logger.TraceFunction(ctx)()

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	query := `DELETE FROM User WHERE id = ?`

	result, err := h.execQuery(ctx, query, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete user: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get rows affected: %v", err)
	}

	if rowsAffected == 0 {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	return &pb.DeleteUserResponse{
		Success: true,
	}, nil
}













// ListUsers lists Users with pagination and filtering
func (h *Handler) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Default pagination
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

	// Calculate offset
	offset := (page - 1) * pageSize

	// Build WHERE clause from filters
	whereClause := ""
	args := []interface{}{}
	whiteMap := map[string]bool{
		"name": true,
		"status": true,
		
	}
	if req.Search != nil && len(req.Search.Filters) > 0 {
		whereConditions := []string{}
		for _, filter := range req.Search.Filters {
			if filter.GetCondition() != nil {
				condition := filter.GetCondition()
				if _, ok := whiteMap[condition.Field]; !ok {
					continue
				}
				whereConditions = append(whereConditions, helper.BuildFilterCondition(condition, &args))
			}
		}
		if len(whereConditions) > 0 {
			whereClause = "WHERE " + strings.Join(whereConditions, " AND ")
		}
	}

	// Build ORDER BY clause
	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	// Get total count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM User %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count users: %v", err)
	}

	// Get entities with pagination
	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, name, status, created_at, updated_at, created_by, updated_by
		FROM User
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
		var entity pb.User
		var createdAt, updatedAt sql.NullTime
		var createdBy, updatedBy sql.NullString
		var StatusStr string
		
		err := rows.Scan(
			&entity.Id,
			&entity.Name,
			&StatusStr,
			&createdAt,
			&updatedAt,
			&createdBy,
			&updatedBy,
		)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan user: %v", err)
		}

		// Convert Status string to enum
		switch StatusStr {
		case "active":
			entity.Status = pb.UserStatus_ACTIVE
		case "inactive":
			entity.Status = pb.UserStatus_INACTIVE
		default:
			entity.Status = pb.UserStatus_ACTIVE
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

		entities = append(entities, &entity)
	}

	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating users: %v", err)
	}

	return &pb.ListUsersResponse{
		Users: entities,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}


