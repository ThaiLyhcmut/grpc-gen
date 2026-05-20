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

// scanCommunityReport reads a single row into a *pb.CommunityReport.
// Shared by Create/Update inline-select and the List handler.
func scanCommunityReport(scanner interface{ Scan(...interface{}) error }) (*pb.CommunityReport, error) {
	var entity pb.CommunityReport
	var createdAt, updatedAt sql.NullTime
	var createdBy, updatedBy sql.NullString
	var TargetTypeStr string
	var StatusStr string
	var ResolvedAtTime sql.NullTime
	var NoteNull sql.NullString
	var ResolvedByAdminIdNull sql.NullInt64

	err := scanner.Scan(
		&entity.Id,
		&entity.ReporterUserId,
		&TargetTypeStr,
		&entity.TargetId,
		&entity.Reason,
		&NoteNull,
		&StatusStr,
		&ResolvedByAdminIdNull,
		&ResolvedAtTime,
		&createdAt,
		&updatedAt,
		&createdBy,
		&updatedBy,
	)
	if err != nil {
		return nil, err
	}

	switch TargetTypeStr {
	case "report_target_type_unspecified":
		entity.TargetType = pb.ReportTargetType_REPORT_TARGET_TYPE_UNSPECIFIED
	case "tgt_material":
		entity.TargetType = pb.ReportTargetType_TGT_MATERIAL
	case "tgt_exam":
		entity.TargetType = pb.ReportTargetType_TGT_EXAM
	default:
		entity.TargetType = pb.ReportTargetType_REPORT_TARGET_TYPE_UNSPECIFIED
	}
	switch StatusStr {
	case "report_status_unspecified":
		entity.Status = pb.ReportStatus_REPORT_STATUS_UNSPECIFIED
	case "rpt_open":
		entity.Status = pb.ReportStatus_RPT_OPEN
	case "rpt_dismissed":
		entity.Status = pb.ReportStatus_RPT_DISMISSED
	case "rpt_actioned":
		entity.Status = pb.ReportStatus_RPT_ACTIONED
	default:
		entity.Status = pb.ReportStatus_REPORT_STATUS_UNSPECIFIED
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
	if ResolvedAtTime.Valid {
		entity.ResolvedAt = timestamppb.New(ResolvedAtTime.Time)
	}
	if NoteNull.Valid {
		val := NoteNull.String
		entity.Note = &val
	}
	if ResolvedByAdminIdNull.Valid {
		val := uint64(ResolvedByAdminIdNull.Int64)
		entity.ResolvedByAdminId = &val
	}

	return &entity, nil
}

// buildCommunityReportWhere assembles a WHERE clause from FilterCriteria using
// the whitelist defined in communityreport_filterable.go. Supports nested
// FilterGroup (AND/OR) via helper recursion.
//
// Mode is controlled by FILTER_STRICT env (default = strict):
//   - strict (default): unknown field → InvalidArgument listing every rejected
//     field across the whole filter tree.
//   - FILTER_STRICT=false: unknown fields are silently dropped (legacy).
func buildCommunityReportWhere(filters []*commonpb.FilterCriteria) (string, []interface{}, error) {
	args := []interface{}{}
	if os.Getenv("FILTER_STRICT") == "false" {
		return helper.BuildWhereClause(filters, &args, CommunityReportFilterableFields), args, nil
	}
	clause, err := helper.BuildWhereClauseStrict(filters, &args, CommunityReportFilterableFields)
	return clause, args, err
}

// CreateCommunityReport creates a new CommunityReport record.
//
// ID handling depends on the entity's `id` type and whether CreateRequest
// declares an `optional id` field:
//   - string id: if client supplies a non-empty value, use it; otherwise the
//     server generates a UUID. Useful for slug-style IDs (e.g. "tin-tuc-foo").
//   - integer id (int32/int64/uint32/uint64): if client supplies a non-zero
//     value, use it; otherwise the column is left to MySQL AUTO_INCREMENT
//     and the inserted ID is recovered via LastInsertId().
func (h *Handler) CreateCommunityReport(ctx context.Context, req *pb.CreateCommunityReportRequest) (*pb.CreateCommunityReportResponse, error) {
	defer logger.TraceFunction(ctx)()

	// Validate required fields (only string types)
	if req.Reason == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
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
	// Optional uint64: ResolvedByAdminId
	var ResolvedByAdminId interface{}
	if req.ResolvedByAdminId != nil {
		ResolvedByAdminId = *req.ResolvedByAdminId
	}
	// Optional timestamp: ResolvedAt
	var ResolvedAt interface{}
	if req.ResolvedAt != nil {
		ResolvedAt = req.ResolvedAt.AsTime()
	}

	// Convert TargetType enum to string
	TargetTypeValue := pb.ReportTargetType_REPORT_TARGET_TYPE_UNSPECIFIED

	TargetTypeValue = req.TargetType
	TargetTypeStr := "report_target_type_unspecified"
	switch TargetTypeValue {
	case pb.ReportTargetType_REPORT_TARGET_TYPE_UNSPECIFIED:
		TargetTypeStr = "report_target_type_unspecified"
	case pb.ReportTargetType_TGT_MATERIAL:
		TargetTypeStr = "tgt_material"
	case pb.ReportTargetType_TGT_EXAM:
		TargetTypeStr = "tgt_exam"
	}
	// Convert Status enum to string
	StatusValue := pb.ReportStatus_REPORT_STATUS_UNSPECIFIED

	StatusValue = req.Status
	StatusStr := "report_status_unspecified"
	switch StatusValue {
	case pb.ReportStatus_REPORT_STATUS_UNSPECIFIED:
		StatusStr = "report_status_unspecified"
	case pb.ReportStatus_RPT_OPEN:
		StatusStr = "rpt_open"
	case pb.ReportStatus_RPT_DISMISSED:
		StatusStr = "rpt_dismissed"
	case pb.ReportStatus_RPT_ACTIONED:
		StatusStr = "rpt_actioned"
	}

	// Handle created_by field
	createdBy := req.CreatedBy

	query := `
		INSERT INTO communityreport (id, reporter_user_id, target_type, target_id, reason, note, status, resolved_by_admin_id, resolved_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := h.execQuery(ctx, query,
		idArg,
		req.ReporterUserId,
		TargetTypeStr,
		req.TargetId,
		req.Reason,
		Note,
		StatusStr,
		ResolvedByAdminId,
		ResolvedAt,
		createdBy,
	)

	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, status.Error(codes.AlreadyExists, "communityreport already exists")
		}
		return nil, status.Errorf(codes.Internal, "failed to create communityreport: %v", err)
	}

	// Recover AUTO_INCREMENT value when the caller didn't supply an id.
	if id == 0 {
		insertedID, err := result.LastInsertId()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read inserted id: %v", err)
		}
		id = uint64(insertedID)
	}

	// Inline SELECT to return the created entity (replaces previous h.GetCommunityReport call).
	selectQuery := `
		SELECT id, reporter_user_id, target_type, target_id, reason, note, status, resolved_by_admin_id, resolved_at, created_at, updated_at, created_by, updated_by
		FROM communityreport
		WHERE id = ?
	`
	entity, err := scanCommunityReport(h.queryRow(ctx, selectQuery, id))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch created communityreport: %v", err)
	}

	return &pb.CreateCommunityReportResponse{
		CommunityReport: entity,
	}, nil
}

// UpdateCommunityReport applies the request field changes to ALL rows matching Filters.
// Returns the updated rows and affected_count.
func (h *Handler) UpdateCommunityReport(ctx context.Context, req *pb.UpdateCommunityReportRequest) (*pb.UpdateCommunityReportResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildCommunityReportWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for update (empty filter would update all rows)")
	}

	// Build dynamic SET clause from request fields
	updateFields := []string{}
	args := []interface{}{}

	// Optional field: Status
	if req.Status != nil {
		updateFields = append(updateFields, "status = ?")
		StatusStr := "report_status_unspecified"
		switch *req.Status {
		case pb.ReportStatus_REPORT_STATUS_UNSPECIFIED:
			StatusStr = "report_status_unspecified"
		case pb.ReportStatus_RPT_OPEN:
			StatusStr = "rpt_open"
		case pb.ReportStatus_RPT_DISMISSED:
			StatusStr = "rpt_dismissed"
		case pb.ReportStatus_RPT_ACTIONED:
			StatusStr = "rpt_actioned"
		}
		args = append(args, StatusStr)

	}
	// Optional field: ResolvedByAdminId
	if req.ResolvedByAdminId != nil {
		updateFields = append(updateFields, "resolved_by_admin_id = ?")
		args = append(args, *req.ResolvedByAdminId)

	}
	// Optional field: ResolvedAt
	if req.ResolvedAt != nil {
		updateFields = append(updateFields, "resolved_at = ?")
		args = append(args, req.ResolvedAt.AsTime())

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

	query := fmt.Sprintf(`UPDATE communityreport SET %s %s`,
		strings.Join(updateFields, ", "), whereClause)

	result, err := h.execQuery(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update communityreport: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	// SELECT back the updated rows so the client gets the current state.
	selectQuery := fmt.Sprintf(`SELECT id, reporter_user_id, target_type, target_id, reason, note, status, resolved_by_admin_id, resolved_at, created_at, updated_at, created_by, updated_by FROM communityreport %s`, whereClause)
	rows, err := h.query(ctx, selectQuery, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read back updated communityreports: %v", err)
	}
	defer rows.Close()

	entities := []*pb.CommunityReport{}
	for rows.Next() {
		entity, err := scanCommunityReport(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan communityreport: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating communityreports: %v", err)
	}

	return &pb.UpdateCommunityReportResponse{
		CommunityReport: entities,
		AffectedCount:   int32(affected),
	}, nil
}

// DeleteCommunityReport deletes ALL rows matching Filters. Empty filter is rejected.
func (h *Handler) DeleteCommunityReport(ctx context.Context, req *pb.DeleteCommunityReportRequest) (*pb.DeleteCommunityReportResponse, error) {
	defer logger.TraceFunction(ctx)()

	whereClause, whereArgs, err := buildCommunityReportWhere(req.GetFilters())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if whereClause == "" {
		return nil, status.Error(codes.InvalidArgument, "filters are required for delete (empty filter would delete all rows)")
	}

	query := fmt.Sprintf(`DELETE FROM communityreport %s`, whereClause)

	result, err := h.execQuery(ctx, query, whereArgs...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete communityreport: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read rows affected: %v", err)
	}

	return &pb.DeleteCommunityReportResponse{
		AffectedCount: int32(affected),
	}, nil
}

// ListCommunityReport lists CommunityReports with pagination and filtering
func (h *Handler) ListCommunityReport(ctx context.Context, req *pb.ListCommunityReportRequest) (*pb.ListCommunityReportResponse, error) {
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
		whereClause, args, err = buildCommunityReportWhere(req.Search.Filters)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	sortDirection := "ASC"
	if descending {
		sortDirection = "DESC"
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM communityreport %s", whereClause)
	var total int32
	err := h.queryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count communityreports: %v", err)
	}

	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, reporter_user_id, target_type, target_id, reason, note, status, resolved_by_admin_id, resolved_at, created_at, updated_at, created_by, updated_by
		FROM communityreport
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortDirection)

	rows, err := h.query(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list communityreports: %v", err)
	}
	defer rows.Close()

	entities := []*pb.CommunityReport{}
	for rows.Next() {
		entity, err := scanCommunityReport(rows)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to scan communityreport: %v", err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "error iterating communityreports: %v", err)
	}

	return &pb.ListCommunityReportResponse{
		CommunityReport: entities,
		Total:           total,
		Page:            page,
		PageSize:        pageSize,
	}, nil
}
