package handler

import (
	"context"
	"database/sql"
	"fmt"
	commonpb "github.com/thaily/lms/proto/common"
	pb "github.com/thaily/lms/proto/exam"
	"github.com/thaily/lms/src/service/pkg/helper"
	"github.com/thaily/lms/src/service/pkg/logger"
	"os"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// scanExam reads a single row into a *pb.Exam.
// Shared by Create/Update inline-select and the List handler.
func scanExam(scanner interface{ Scan(...interface{}) error }) (*pb.Exam, error) {
	var entity pb.Exam
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var VisibilityStr string
	var CommunityStatusStr string
	var ShowResultModeStr string
	var DescriptionNull sql.NullString
	var SubjectNull sql.NullString
	var RejectReasonNull sql.NullString

	err := scanner.Scan(
		&entity.Id,
		&entity.OwnerTeacherId,
		&entity.Title,
		&DescriptionNull,
		&SubjectNull,
		&entity.DurationMin,
		&entity.TotalPoints,
		&VisibilityStr,
		&CommunityStatusStr,
		&RejectReasonNull,
		&entity.ShuffleQuestions,
		&entity.ShuffleChoices,
		&ShowResultModeStr,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch VisibilityStr {
	case "exam_visibility_unspecified":
		entity.Visibility = pb.ExamVisibility_EXAM_VISIBILITY_UNSPECIFIED
	case "exam_class":
		entity.Visibility = pb.ExamVisibility_EXAM_CLASS
	case "exam_community":
		entity.Visibility = pb.ExamVisibility_EXAM_COMMUNITY
	default:
		entity.Visibility = pb.ExamVisibility_EXAM_VISIBILITY_UNSPECIFIED
	}
	// Optional enum CommunityStatus: assign via pointer so unset/NULL stays nil.
	var CommunityStatusVal pb.ExamCommunityStatus
	switch CommunityStatusStr {
	case "exam_community_status_unspecified":
		CommunityStatusVal = pb.ExamCommunityStatus_EXAM_COMMUNITY_STATUS_UNSPECIFIED
	case "exam_draft":
		CommunityStatusVal = pb.ExamCommunityStatus_EXAM_DRAFT
	case "exam_pending":
		CommunityStatusVal = pb.ExamCommunityStatus_EXAM_PENDING
	case "exam_approved":
		CommunityStatusVal = pb.ExamCommunityStatus_EXAM_APPROVED
	case "exam_rejected":
		CommunityStatusVal = pb.ExamCommunityStatus_EXAM_REJECTED
	default:
		CommunityStatusVal = pb.ExamCommunityStatus_EXAM_COMMUNITY_STATUS_UNSPECIFIED
	}
	entity.CommunityStatus = &CommunityStatusVal
	switch ShowResultModeStr {
	case "show_result_mode_unspecified":
		entity.ShowResultMode = pb.ShowResultMode_SHOW_RESULT_MODE_UNSPECIFIED
	case "immediately":
		entity.ShowResultMode = pb.ShowResultMode_IMMEDIATELY
	case "after_close":
		entity.ShowResultMode = pb.ShowResultMode_AFTER_CLOSE
	case "manual":
		entity.ShowResultMode = pb.ShowResultMode_MANUAL
	default:
		entity.ShowResultMode = pb.ShowResultMode_SHOW_RESULT_MODE_UNSPECIFIED
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
	if DescriptionNull.Valid {
		val := DescriptionNull.String
		entity.Description = &val
	}
	if SubjectNull.Valid {
		val := SubjectNull.String
		entity.Subject = &val
	}
	if RejectReasonNull.Valid {
		val := RejectReasonNull.String
		entity.RejectReason = &val
	}

	return &entity, nil
}

// buildExamWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in exam_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildExamWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, ExamFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, ExamFilterableFields)
	return clause, args, err
}

// CreateExam creates a new Exam record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateExam(ctx context.Context, req *pb.CreateExamRequest) (*pb.CreateExamResponse, error) {
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
	// Optional string: Subject
	var Subject interface{}
	if req.Subject != nil {
		Subject = *req.Subject
	}
	// Optional string: RejectReason
	var RejectReason interface{}
	if req.RejectReason != nil {
		RejectReason = *req.RejectReason
	}

	// Convert Visibility enum to string
	VisibilityValue := pb.ExamVisibility_EXAM_VISIBILITY_UNSPECIFIED

	VisibilityValue = req.Visibility
	VisibilityStr := "exam_visibility_unspecified"
	switch VisibilityValue {
	case pb.ExamVisibility_EXAM_VISIBILITY_UNSPECIFIED:
		VisibilityStr = "exam_visibility_unspecified"
	case pb.ExamVisibility_EXAM_CLASS:
		VisibilityStr = "exam_class"
	case pb.ExamVisibility_EXAM_COMMUNITY:
		VisibilityStr = "exam_community"
	}
	// Convert CommunityStatus enum to string
	CommunityStatusValue := pb.ExamCommunityStatus_EXAM_COMMUNITY_STATUS_UNSPECIFIED
	if req.CommunityStatus != nil {
		CommunityStatusValue = *req.CommunityStatus
	}
	CommunityStatusStr := "exam_community_status_unspecified"
	switch CommunityStatusValue {
	case pb.ExamCommunityStatus_EXAM_COMMUNITY_STATUS_UNSPECIFIED:
		CommunityStatusStr = "exam_community_status_unspecified"
	case pb.ExamCommunityStatus_EXAM_DRAFT:
		CommunityStatusStr = "exam_draft"
	case pb.ExamCommunityStatus_EXAM_PENDING:
		CommunityStatusStr = "exam_pending"
	case pb.ExamCommunityStatus_EXAM_APPROVED:
		CommunityStatusStr = "exam_approved"
	case pb.ExamCommunityStatus_EXAM_REJECTED:
		CommunityStatusStr = "exam_rejected"
	}
	// Convert ShowResultMode enum to string
	ShowResultModeValue := pb.ShowResultMode_SHOW_RESULT_MODE_UNSPECIFIED

	ShowResultModeValue = req.ShowResultMode
	ShowResultModeStr := "show_result_mode_unspecified"
	switch ShowResultModeValue {
	case pb.ShowResultMode_SHOW_RESULT_MODE_UNSPECIFIED:
		ShowResultModeStr = "show_result_mode_unspecified"
	case pb.ShowResultMode_IMMEDIATELY:
		ShowResultModeStr = "immediately"
	case pb.ShowResultMode_AFTER_CLOSE:
		ShowResultModeStr = "after_close"
	case pb.ShowResultMode_MANUAL:
		ShowResultModeStr = "manual"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO exam (id, owner_teacher_id, title, description, subject, duration_min, total_points, visibility, community_status, reject_reason, shuffle_questions, shuffle_choices, show_result_mode, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.OwnerTeacherId,
		req.Title,
		Description,
		Subject,
		req.DurationMin,
		req.TotalPoints,
		VisibilityStr,
		CommunityStatusStr,
		RejectReason,
		req.ShuffleQuestions,
		req.ShuffleChoices,
		ShowResultModeStr,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "exam already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create exam: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetExam call).
	selectQuery := `
		SELECT id, owner_teacher_id, title, description, subject, duration_min, total_points, visibility, community_status, reject_reason, shuffle_questions, shuffle_choices, show_result_mode, created_at, updated_at, created_by, updated_by
		FROM exam
		WHERE id = ?
	`
	entity, err := scanExam(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created exam: %v", err)
	}

	return &pb.CreateExamResponse{
		Exam: entity,
	}, nil
}

// UpdateExam applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateExam(ctx context.Context, req *pb.UpdateExamRequest) (*pb.UpdateExamResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildExamWhere(req.GetFilters())
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
	// Optional field: Subject
	if req.Subject != nil {
		updateFields = append(updateFields, "subject = ?")
		args = append(args, *req.Subject)

	}
	// Optional field: DurationMin
	if req.DurationMin != nil {
		updateFields = append(updateFields, "duration_min = ?")
		args = append(args, *req.DurationMin)

	}
	// Optional field: TotalPoints
	if req.TotalPoints != nil {
		updateFields = append(updateFields, "total_points = ?")
		args = append(args, *req.TotalPoints)

	}
	// Optional field: Visibility
	if req.Visibility != nil {
		updateFields = append(updateFields, "visibility = ?")
		VisibilityStr := "exam_visibility_unspecified"
		switch *req.Visibility {
		case pb.ExamVisibility_EXAM_VISIBILITY_UNSPECIFIED:
			VisibilityStr = "exam_visibility_unspecified"
		case pb.ExamVisibility_EXAM_CLASS:
			VisibilityStr = "exam_class"
		case pb.ExamVisibility_EXAM_COMMUNITY:
			VisibilityStr = "exam_community"
		}
		args = append(args, VisibilityStr)

	}
	// Optional field: CommunityStatus
	if req.CommunityStatus != nil {
		updateFields = append(updateFields, "community_status = ?")
		CommunityStatusStr := "exam_community_status_unspecified"
		switch *req.CommunityStatus {
		case pb.ExamCommunityStatus_EXAM_COMMUNITY_STATUS_UNSPECIFIED:
			CommunityStatusStr = "exam_community_status_unspecified"
		case pb.ExamCommunityStatus_EXAM_DRAFT:
			CommunityStatusStr = "exam_draft"
		case pb.ExamCommunityStatus_EXAM_PENDING:
			CommunityStatusStr = "exam_pending"
		case pb.ExamCommunityStatus_EXAM_APPROVED:
			CommunityStatusStr = "exam_approved"
		case pb.ExamCommunityStatus_EXAM_REJECTED:
			CommunityStatusStr = "exam_rejected"
		}
		args = append(args, CommunityStatusStr)

	}
	// Optional field: RejectReason
	if req.RejectReason != nil {
		updateFields = append(updateFields, "reject_reason = ?")
		args = append(args, *req.RejectReason)

	}
	// Optional field: ShuffleQuestions
	if req.ShuffleQuestions != nil {
		updateFields = append(updateFields, "shuffle_questions = ?")
		args = append(args, *req.ShuffleQuestions)

	}
	// Optional field: ShuffleChoices
	if req.ShuffleChoices != nil {
		updateFields = append(updateFields, "shuffle_choices = ?")
		args = append(args, *req.ShuffleChoices)

	}
	// Optional field: ShowResultMode
	if req.ShowResultMode != nil {
		updateFields = append(updateFields, "show_result_mode = ?")
		ShowResultModeStr := "show_result_mode_unspecified"
		switch *req.ShowResultMode {
		case pb.ShowResultMode_SHOW_RESULT_MODE_UNSPECIFIED:
			ShowResultModeStr = "show_result_mode_unspecified"
		case pb.ShowResultMode_IMMEDIATELY:
			ShowResultModeStr = "immediately"
		case pb.ShowResultMode_AFTER_CLOSE:
			ShowResultModeStr = "after_close"
		case pb.ShowResultMode_MANUAL:
			ShowResultModeStr = "manual"
		}
		args = append(args, ShowResultModeStr)

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

	query := fmt.Sprintf(`UPDATE exam SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update exam: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, owner_teacher_id, title, description, subject, duration_min, total_points, visibility, community_status, reject_reason, shuffle_questions, shuffle_choices, show_result_mode, created_at, updated_at, created_by, updated_by FROM exam %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated exams: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Exam{}
	for rows.Next() {
		entity, err := scanExam(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan exam: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating exams: %v", err)
	}

	return &pb.UpdateExamResponse{
		Exam:          entities,
		AffectedCount: int32(affected),
	}, nil
}

// DeleteExam deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteExam(ctx context.Context, req *pb.DeleteExamRequest) (*pb.DeleteExamResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildExamWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM exam %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete exam: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteExamResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListExam lists Exams with pagination and filtering
func (h *Handler) ListExam(ctx context.Context, req *pb.ListExamRequest) (*pb.ListExamResponse, error) {
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
		whereClause, args, err = buildExamWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM exam %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count exams: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, owner_teacher_id, title, description, subject, duration_min, total_points, visibility, community_status, reject_reason, shuffle_questions, shuffle_choices, show_result_mode, created_at, updated_at, created_by, updated_by
		FROM exam
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list exams: %v", err)
	}
	defer rows.Close()

	entities := []*pb.Exam{}
	for rows.Next() {
		entity, err := scanExam(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan exam: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating exams: %v", err)
	}

	return &pb.ListExamResponse{
		Exam:     entities,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
