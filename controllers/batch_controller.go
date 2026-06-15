package controllers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"roster-swap-api/config"
	"roster-swap-api/middleware"
	"roster-swap-api/models"
	"roster-swap-api/utils"

	"github.com/gin-gonic/gin"
)

type BatchApproveRequest struct {
	IDs    []uint `json:"ids" binding:"required"`
	Remark string `json:"remark"`
}

type BatchResult struct {
	SuccessIDs []uint   `json:"success_ids"`
	FailedIDs  []uint   `json:"failed_ids"`
	Errors     []string `json:"errors"`
	TotalCount int      `json:"total_count"`
	SuccessCount int    `json:"success_count"`
	FailedCount  int    `json:"failed_count"`
}

func processSingleApproval(c *gin.Context, swapID uint, approver *models.User, remark string) error {
	var swapReq models.SwapRequest
	if err := config.DB.Preload("Requester").Preload("TargetUser").
		Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, swapID).Error; err != nil {
		return fmt.Errorf("ID %d: 申请不存在", swapID)
	}

	if swapReq.Status != models.SwapStatusAccepted {
		return fmt.Errorf("ID %d: 状态为 %s，仅 accepted 状态可审批", swapID, swapReq.Status)
	}

	conflict, err := detectTimeSlotConflict(
		swapReq.RequesterID,
		swapReq.RequesterShift.ShiftDate,
		swapReq.RequesterShift.StartTime,
		swapReq.RequesterShift.EndTime,
		swapReq.ID,
	)
	if err != nil {
		return fmt.Errorf("ID %d: 并发冲突检测失败", swapID)
	}
	if conflict != nil {
		return fmt.Errorf("ID %d: 存在并发冲突申请(ID:%d)", swapID, conflict.ID)
	}

	conflict, err = detectTimeSlotConflict(
		swapReq.TargetUserID,
		swapReq.TargetShift.ShiftDate,
		swapReq.TargetShift.StartTime,
		swapReq.TargetShift.EndTime,
		swapReq.ID,
	)
	if err != nil {
		return fmt.Errorf("ID %d: 并发冲突检测失败", swapID)
	}
	if conflict != nil {
		return fmt.Errorf("ID %d: 被申请人时段冲突(ID:%d)", swapID, conflict.ID)
	}

	tx := config.DB.Begin()
	now := time.Now()

	if err := tx.Model(&swapReq).Updates(map[string]interface{}{
		"status":          models.SwapStatusApproved,
		"approver_id":     &approver.ID,
		"approval_remark": remark,
		"approved_at":     &now,
	}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ID %d: 更新申请状态失败: %v", swapID, err)
	}

	if err := tx.Model(&swapReq.RequesterShift).
		Update("user_id", swapReq.TargetUserID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ID %d: 交换申请人班次失败: %v", swapID, err)
	}

	if err := tx.Model(&swapReq.TargetShift).
		Update("user_id", swapReq.RequesterID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ID %d: 交换目标班次失败: %v", swapID, err)
	}

	completeNow := time.Now()
	if err := tx.Model(&swapReq).Updates(map[string]interface{}{
		"status":       models.SwapStatusCompleted,
		"completed_at": &completeNow,
	}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ID %d: 标记完成失败: %v", swapID, err)
	}

	tx.Commit()

	detail := fmt.Sprintf("[%s(%s)] 批量审批通过, 备注: %s", approver.Name, approver.Role, remark)
	userID, _ := c.Get("user_id")
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")
	log := &models.OperationLog{
		UserID:        userID.(uint),
		OperationType: models.OpTypeApproveSwap,
		SwapRequestID: &swapReq.ID,
		Detail:        detail,
		IPAddress:     ip,
		UserAgent:     ua,
	}
	config.DB.Create(log)

	detail2 := "批量审批换班完成"
	log2 := &models.OperationLog{
		UserID:        userID.(uint),
		OperationType: models.OpTypeCompleteSwap,
		SwapRequestID: &swapReq.ID,
		Detail:        detail2,
		IPAddress:     ip,
		UserAgent:     ua,
	}
	config.DB.Create(log2)

	reqShiftDesc := utils.BuildShiftDescription(&swapReq.RequesterShift)
	tgtShiftDesc := utils.BuildShiftDescription(&swapReq.TargetShift)
	meta := utils.SwapNotifMetadata{
		RequesterName:  swapReq.Requester.Name,
		TargetUserName: swapReq.TargetUser.Name,
		ApproverName:   approver.Name,
		RequesterShift: reqShiftDesc,
		TargetShift:    tgtShiftDesc,
		Remark:         remark,
	}
	for _, uid := range []uint{swapReq.RequesterID, swapReq.TargetUserID} {
		title := "【批量审批】换班已通过审批"
		content := fmt.Sprintf(
			"经理 %s 批量审批通过，换班已完成：\n  %s ←→ %s\n  审批备注: %s",
			approver.Name, reqShiftDesc, tgtShiftDesc, remark)
		utils.NotifyUser(uid, models.NotifTypeSwapCompleted, title, content, &swapReq.ID, meta)
	}

	return nil
}

func processSingleDisapproval(c *gin.Context, swapID uint, approver *models.User, remark string) error {
	var swapReq models.SwapRequest
	if err := config.DB.Preload("Requester").Preload("TargetUser").
		Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, swapID).Error; err != nil {
		return fmt.Errorf("ID %d: 申请不存在", swapID)
	}

	if swapReq.Status != models.SwapStatusAccepted {
		return fmt.Errorf("ID %d: 状态为 %s，仅 accepted 状态可审批", swapID, swapReq.Status)
	}

	now := time.Now()
	if err := config.DB.Model(&swapReq).Updates(map[string]interface{}{
		"status":          models.SwapStatusDisapproved,
		"approver_id":     &approver.ID,
		"approval_remark": remark,
		"approved_at":     &now,
	}).Error; err != nil {
		return fmt.Errorf("ID %d: 更新驳回状态失败: %v", swapID, err)
	}

	detail := fmt.Sprintf("[%s(%s)] 批量审批驳回, 原因: %s", approver.Name, approver.Role, remark)
	userID, _ := c.Get("user_id")
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")
	log := &models.OperationLog{
		UserID:        userID.(uint),
		OperationType: models.OpTypeDisapproveSwap,
		SwapRequestID: &swapReq.ID,
		Detail:        detail,
		IPAddress:     ip,
		UserAgent:     ua,
	}
	config.DB.Create(log)

	reqShiftDesc := utils.BuildShiftDescription(&swapReq.RequesterShift)
	tgtShiftDesc := utils.BuildShiftDescription(&swapReq.TargetShift)
	meta := utils.SwapNotifMetadata{
		RequesterName:  swapReq.Requester.Name,
		TargetUserName: swapReq.TargetUser.Name,
		ApproverName:   approver.Name,
		RequesterShift: reqShiftDesc,
		TargetShift:    tgtShiftDesc,
		Remark:         remark,
	}
	for _, uid := range []uint{swapReq.RequesterID, swapReq.TargetUserID} {
		title := "【批量审批驳回】换班申请未通过"
		content := fmt.Sprintf(
			"经理 %s 批量审批驳回：\n  %s ←→ %s\n  驳回原因: %s\n  班次已回滚可重新申请。",
			approver.Name, reqShiftDesc, tgtShiftDesc, remark)
		utils.NotifyUser(uid, models.NotifTypeSwapDisapproved, title, content, &swapReq.ID, meta)
	}

	return nil
}

func BatchApproveSwap(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	var approver models.User
	if err := config.DB.First(&approver, userID).Error; err != nil {
		utils.InternalServerError(c, "审批人信息获取失败")
		return
	}
	if approver.Role != models.RoleManager && approver.Role != models.RoleAdmin {
		utils.Forbidden(c, fmt.Sprintf("当前角色(%s)无批量审批权限", approver.Role))
		return
	}

	var req BatchApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	if len(req.IDs) == 0 {
		utils.BadRequest(c, "请至少选择一个申请进行审批")
		return
	}
	if len(req.IDs) > 100 {
		utils.BadRequest(c, "单次批量审批不能超过100个申请")
		return
	}

	result := &BatchResult{
		TotalCount: len(req.IDs),
	}

	for _, id := range req.IDs {
		err := processSingleApproval(c, id, &approver, req.Remark)
		if err != nil {
			result.FailedIDs = append(result.FailedIDs, id)
			result.Errors = append(result.Errors, err.Error())
		} else {
			result.SuccessIDs = append(result.SuccessIDs, id)
		}
	}

	result.SuccessCount = len(result.SuccessIDs)
	result.FailedCount = len(result.FailedIDs)

	utils.SuccessWithMessage(c,
		fmt.Sprintf("批量审批完成: 成功%d个，失败%d个", result.SuccessCount, result.FailedCount),
		result)
}

func BatchDisapproveSwap(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	var approver models.User
	if err := config.DB.First(&approver, userID).Error; err != nil {
		utils.InternalServerError(c, "审批人信息获取失败")
		return
	}
	if approver.Role != models.RoleManager && approver.Role != models.RoleAdmin {
		utils.Forbidden(c, fmt.Sprintf("当前角色(%s)无批量审批权限", approver.Role))
		return
	}

	var req BatchApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	if len(req.IDs) == 0 {
		utils.BadRequest(c, "请至少选择一个申请进行驳回")
		return
	}
	if len(req.IDs) > 100 {
		utils.BadRequest(c, "单次批量驳回不能超过100个申请")
		return
	}
	if req.Remark == "" {
		utils.BadRequest(c, "批量驳回必须填写驳回原因")
		return
	}

	result := &BatchResult{
		TotalCount: len(req.IDs),
	}

	for _, id := range req.IDs {
		err := processSingleDisapproval(c, id, &approver, req.Remark)
		if err != nil {
			result.FailedIDs = append(result.FailedIDs, id)
			result.Errors = append(result.Errors, err.Error())
		} else {
			result.SuccessIDs = append(result.SuccessIDs, id)
		}
	}

	result.SuccessCount = len(result.SuccessIDs)
	result.FailedCount = len(result.FailedIDs)

	utils.SuccessWithMessage(c,
		fmt.Sprintf("批量驳回完成: 成功%d个，失败%d个", result.SuccessCount, result.FailedCount),
		result)
}

func ExportOperationLogs(c *gin.Context) {
	role := middleware.GetCurrentUserRole(c)
	userID := middleware.GetCurrentUserID(c)

	if role != "manager" && role != "admin" {
		utils.Forbidden(c, "只有经理或管理员可以导出审计日志")
		return
	}

	targetUserID := c.Query("user_id")
	opType := c.Query("type")
	swapID := c.Query("swap_id")
	shiftID := c.Query("shift_id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	format := c.DefaultQuery("format", "csv")
	if format != "csv" {
		format = "csv"
	}

	query := config.DB.Model(&models.OperationLog{}).Preload("User")

	if targetUserID != "" {
		query = query.Where("user_id = ?", targetUserID)
	}
	if opType != "" {
		query = query.Where("operation_type = ?", opType)
	}
	if swapID != "" {
		query = query.Where("swap_request_id = ?", swapID)
	}
	if shiftID != "" {
		query = query.Where("shift_id = ?", shiftID)
	}
	if startDate != "" {
		query = query.Where("DATE(created_at) >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("DATE(created_at) <= ?", endDate)
	}

	var logs []models.OperationLog
	if err := query.Order("created_at DESC").Find(&logs).Error; err != nil {
		utils.InternalServerError(c, "查询日志失败: "+err.Error())
		return
	}

	var user models.User
	config.DB.First(&user, userID)

	filename := fmt.Sprintf("operation_logs_%s_%s.csv",
		user.Username,
		time.Now().Format("20060102_150405"))

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Writer.WriteString("\xEF\xBB\xBF")

	writer := csv.NewWriter(c.Writer)
	writer.Write([]string{
		"记录ID",
		"操作人ID",
		"操作人姓名",
		"操作人部门",
		"操作类型",
		"换班申请ID",
		"班次ID",
		"操作详情",
		"IP地址",
		"浏览器",
		"操作时间",
	})

	for _, log := range logs {
		opTypeCN := operationTypeCN(log.OperationType)
		userName := ""
		userDept := ""
		if log.User.ID > 0 {
			userName = log.User.Name
			userDept = log.User.Department
		}
		swapStr := ""
		if log.SwapRequestID != nil {
			swapStr = strconv.FormatUint(uint64(*log.SwapRequestID), 10)
		}
		shiftStr := ""
		if log.ShiftID != nil {
			shiftStr = strconv.FormatUint(uint64(*log.ShiftID), 10)
		}

		writer.Write([]string{
			strconv.FormatUint(uint64(log.ID), 10),
			strconv.FormatUint(uint64(log.UserID), 10),
			userName,
			userDept,
			opTypeCN,
			swapStr,
			shiftStr,
			log.Detail,
			log.IPAddress,
			log.UserAgent,
			log.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	writer.Flush()

	detail := fmt.Sprintf("[%s] 导出操作日志，共%d条记录", user.Name, len(logs))
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")
	logEntry := &models.OperationLog{
		UserID:        userID,
		OperationType: "export_logs",
		Detail:        detail,
		IPAddress:     ip,
		UserAgent:     ua,
	}
	config.DB.Create(logEntry)

	c.Status(http.StatusOK)
}

func operationTypeCN(opType models.OperationType) string {
	switch opType {
	case models.OpTypeCreateSwap:
		return "发起换班申请"
	case models.OpTypeAcceptSwap:
		return "同意换班申请"
	case models.OpTypeRejectSwap:
		return "拒绝换班申请"
	case models.OpTypeApproveSwap:
		return "审批通过换班"
	case models.OpTypeDisapproveSwap:
		return "审批驳回换班"
	case models.OpTypeCancelSwap:
		return "取消换班申请"
	case models.OpTypeCompleteSwap:
		return "换班完成"
	case models.OpTypeCreateShift:
		return "创建班次"
	case models.OpTypeUpdateShift:
		return "更新班次"
	case models.OpTypeDeleteShift:
		return "删除班次"
	default:
		return string(opType)
	}
}
