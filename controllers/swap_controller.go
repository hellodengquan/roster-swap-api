package controllers

import (
	"fmt"
	"strconv"
	"time"

	"roster-swap-api/config"
	"roster-swap-api/middleware"
	"roster-swap-api/models"
	"roster-swap-api/utils"

	"github.com/gin-gonic/gin"
)

type CreateSwapReq struct {
	TargetUserID     uint   `json:"target_user_id" binding:"required"`
	RequesterShiftID uint   `json:"requester_shift_id" binding:"required"`
	TargetShiftID    uint   `json:"target_shift_id" binding:"required"`
	Reason           string `json:"reason" binding:"required,max=500"`
}

type RespondSwapReq struct {
	Remark string `json:"remark"`
}

type ApproveSwapReq struct {
	Remark string `json:"remark"`
}

func detectTimeSlotConflict(userID uint, shiftDate time.Time, startTime, endTime string, excludeSwapIDs ...uint) (*models.SwapRequest, error) {
	var userShifts []models.Shift
	if err := config.DB.Where("user_id = ? AND shift_date = ?", userID, shiftDate).Find(&userShifts).Error; err != nil {
		return nil, err
	}

	var shiftIDs []uint
	for _, s := range userShifts {
		shiftIDs = append(shiftIDs, s.ID)
	}
	if len(shiftIDs) == 0 {
		return nil, nil
	}

	activeStatuses := []string{
		string(models.SwapStatusPending),
		string(models.SwapStatusAccepted),
	}

	query := config.DB.Where(
		`(requester_id = ? AND requester_shift_id IN ? AND status IN ?) OR
		(requester_id = ? AND target_shift_id IN ? AND status IN ?) OR
		(target_user_id = ? AND requester_shift_id IN ? AND status IN ?) OR
		(target_user_id = ? AND target_shift_id IN ? AND status IN ?)`,
		userID, shiftIDs, activeStatuses,
		userID, shiftIDs, activeStatuses,
		userID, shiftIDs, activeStatuses,
		userID, shiftIDs, activeStatuses,
	)

	if len(excludeSwapIDs) > 0 {
		query = query.Where("id NOT IN ?", excludeSwapIDs)
	}

	var conflictSwap models.SwapRequest
	err := query.Preload("Requester").Preload("TargetUser").
		Preload("RequesterShift").Preload("TargetShift").
		First(&conflictSwap).Error

	if err != nil {
		return nil, nil
	}
	return &conflictSwap, nil
}

func CreateSwapRequest(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	var req CreateSwapReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	if req.TargetUserID == userID {
		utils.BadRequest(c, "不能与自己换班")
		return
	}

	var requesterShift models.Shift
	if err := config.DB.First(&requesterShift, req.RequesterShiftID).Error; err != nil {
		utils.NotFound(c, "申请人班次不存在")
		return
	}
	if requesterShift.UserID != userID {
		utils.BadRequest(c, "只能申请自己的班次")
		return
	}
	if requesterShift.Status != models.ShiftStatusActive {
		utils.BadRequest(c, "该班次状态不可用于换班")
		return
	}

	var targetShift models.Shift
	if err := config.DB.First(&targetShift, req.TargetShiftID).Error; err != nil {
		utils.NotFound(c, "目标班次不存在")
		return
	}
	if targetShift.UserID != req.TargetUserID {
		utils.BadRequest(c, "目标班次不属于目标用户")
		return
	}
	if targetShift.Status != models.ShiftStatusActive {
		utils.BadRequest(c, "目标班次状态不可用于换班")
		return
	}

	var directConflict models.SwapRequest
	sql := `(requester_id = ? AND requester_shift_id = ? AND status IN ('pending', 'accepted')) OR
		(requester_id = ? AND target_shift_id = ? AND status IN ('pending', 'accepted')) OR
		(target_user_id = ? AND requester_shift_id = ? AND status IN ('pending', 'accepted')) OR
		(target_user_id = ? AND target_shift_id = ? AND status IN ('pending', 'accepted'))`
	params := []interface{}{
		userID, req.RequesterShiftID,
		userID, req.TargetShiftID,
		userID, req.RequesterShiftID,
		userID, req.TargetShiftID,
	}
	err := config.DB.Where(sql, params...).First(&directConflict).Error
	if err == nil {
		utils.BadRequest(c, fmt.Sprintf("该班次已有进行中的换班申请(申请ID: %d)", directConflict.ID))
		return
	}

	conflict, err := detectTimeSlotConflict(userID, requesterShift.ShiftDate, requesterShift.StartTime, requesterShift.EndTime)
	if err != nil {
		utils.InternalServerError(c, "并发冲突检测失败: "+err.Error())
		return
	}
	if conflict != nil {
		utils.BadRequest(c, fmt.Sprintf(
			"申请人在该时段存在冲突换班申请(ID:%d, 当前状态:%s)，请先处理该申请再发起新申请",
			conflict.ID, conflict.Status,
		))
		return
	}

	conflict, err = detectTimeSlotConflict(req.TargetUserID, targetShift.ShiftDate, targetShift.StartTime, targetShift.EndTime)
	if err != nil {
		utils.InternalServerError(c, "并发冲突检测失败: "+err.Error())
		return
	}
	if conflict != nil {
		utils.BadRequest(c, fmt.Sprintf(
			"被申请人在该时段存在冲突换班申请(ID:%d, 当前状态:%s)，请选择其他时段",
			conflict.ID, conflict.Status,
		))
		return
	}

	swapReq := &models.SwapRequest{
		RequesterID:      userID,
		TargetUserID:     req.TargetUserID,
		RequesterShiftID: req.RequesterShiftID,
		TargetShiftID:    req.TargetShiftID,
		Reason:           req.Reason,
		Status:           models.SwapStatusPending,
	}

	if err := config.DB.Create(swapReq).Error; err != nil {
		utils.InternalServerError(c, "创建换班申请失败: "+err.Error())
		return
	}

	if err := config.DB.Preload("Requester").Preload("TargetUser").
		Preload("RequesterShift").Preload("TargetShift").
		First(swapReq, swapReq.ID).Error; err == nil {
	}

	detail := fmt.Sprintf("发起换班申请: 我的班次[%s] <-> 对方班次[%s], 原因: %s",
		utils.BuildShiftDescription(&requesterShift),
		utils.BuildShiftDescription(&targetShift),
		req.Reason)
	middleware.LogOperation(c, models.OpTypeCreateSwap, &swapReq.ID, nil, detail)

	reqShiftDesc := utils.BuildShiftDescription(&requesterShift)
	tgtShiftDesc := utils.BuildShiftDescription(&targetShift)
	meta := utils.SwapNotifMetadata{
		RequesterName:  swapReq.Requester.Name,
		TargetUserName: swapReq.TargetUser.Name,
		RequesterShift: reqShiftDesc,
		TargetShift:    tgtShiftDesc,
		Reason:         req.Reason,
	}
	targetTitle := fmt.Sprintf("【换班申请】%s 申请与你换班", swapReq.Requester.Name)
	targetContent := fmt.Sprintf("%s(%s) 发起换班申请：\n  对方班次: %s\n  你的班次: %s\n  原因: %s\n  请及时登录系统处理。",
		swapReq.Requester.Name, swapReq.Requester.Department,
		reqShiftDesc, tgtShiftDesc, req.Reason)
	utils.NotifyUser(swapReq.TargetUserID, models.NotifTypeSwapCreated, targetTitle, targetContent, &swapReq.ID, meta)

	reqTitle := "【换班申请】申请已提交"
	reqContent := fmt.Sprintf("您的换班申请已提交，等待 %s(%s) 确认。\n  对方班次: %s\n  你的班次: %s",
		swapReq.TargetUser.Name, swapReq.TargetUser.Department, tgtShiftDesc, reqShiftDesc)
	utils.NotifyUser(userID, models.NotifTypeSwapCreated, reqTitle, reqContent, &swapReq.ID, meta)

	utils.Success(c, swapReq)
}

func GetSwapRequestList(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)
	role := middleware.GetCurrentUserRole(c)

	status := c.Query("status")
	asRequester := c.Query("as_requester")
	asTarget := c.Query("as_target")

	query := config.DB.Model(&models.SwapRequest{}).
		Preload("Requester").Preload("TargetUser").Preload("Approver").
		Preload("RequesterShift").Preload("TargetShift")

	if role == "employee" {
		if asRequester == "true" {
			query = query.Where("requester_id = ?", userID)
		} else if asTarget == "true" {
			query = query.Where("target_user_id = ?", userID)
		} else {
			query = query.Where("requester_id = ? OR target_user_id = ?", userID, userID)
		}
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	var swapRequests []models.SwapRequest
	if err := query.Order("created_at DESC").Find(&swapRequests).Error; err != nil {
		utils.InternalServerError(c, "获取换班申请列表失败")
		return
	}

	utils.Success(c, swapRequests)
}

func GetSwapRequest(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的申请ID")
		return
	}

	var swapReq models.SwapRequest
	if err := config.DB.Preload("Requester").Preload("TargetUser").Preload("Approver").
		Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, uint(id)).Error; err != nil {
		utils.NotFound(c, "换班申请不存在")
		return
	}

	utils.Success(c, swapReq)
}

func AcceptSwapRequest(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的申请ID")
		return
	}

	var req RespondSwapReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	var swapReq models.SwapRequest
	if err := config.DB.Preload("Requester").Preload("TargetUser").
		Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, uint(id)).Error; err != nil {
		utils.NotFound(c, "换班申请不存在")
		return
	}

	if swapReq.TargetUserID != userID {
		utils.Forbidden(c, "只有被申请人可以确认")
		return
	}

	if swapReq.Status != models.SwapStatusPending {
		utils.BadRequest(c, "当前状态不允许确认，状态为: "+string(swapReq.Status))
		return
	}

	conflict, err := detectTimeSlotConflict(
		swapReq.RequesterID,
		swapReq.RequesterShift.ShiftDate,
		swapReq.RequesterShift.StartTime,
		swapReq.RequesterShift.EndTime,
		swapReq.ID,
	)
	if err != nil {
		utils.InternalServerError(c, "并发冲突检测失败")
		return
	}
	if conflict != nil {
		utils.BadRequest(c, fmt.Sprintf(
			"申请人在该时段新增了冲突换班申请(ID:%d)，请先取消确认后重试",
			conflict.ID,
		))
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":             models.SwapStatusAccepted,
		"target_user_remark": req.Remark,
		"target_user_at":     &now,
	}

	if err := config.DB.Model(&swapReq).Updates(updates).Error; err != nil {
		utils.InternalServerError(c, "确认失败: "+err.Error())
		return
	}

	detail := fmt.Sprintf("同意换班申请，备注: %s", req.Remark)
	middleware.LogOperation(c, models.OpTypeAcceptSwap, &swapReq.ID, nil, detail)

	reqShiftDesc := utils.BuildShiftDescription(&swapReq.RequesterShift)
	tgtShiftDesc := utils.BuildShiftDescription(&swapReq.TargetShift)
	meta := utils.SwapNotifMetadata{
		RequesterName:  swapReq.Requester.Name,
		TargetUserName: swapReq.TargetUser.Name,
		RequesterShift: reqShiftDesc,
		TargetShift:    tgtShiftDesc,
		Remark:         req.Remark,
	}
	notifyTitle := fmt.Sprintf("【换班确认】%s 已同意你的换班申请", swapReq.TargetUser.Name)
	notifyContent := fmt.Sprintf("%s 已同意换班申请：\n  你的班次: %s\n  对方班次: %s\n  备注: %s\n  等待经理审批中。",
		swapReq.TargetUser.Name, reqShiftDesc, tgtShiftDesc, req.Remark)
	utils.NotifyUser(swapReq.RequesterID, models.NotifTypeSwapAccepted, notifyTitle, notifyContent, &swapReq.ID, meta)

	utils.SuccessWithMessage(c, "已确认换班申请，已通知申请人等待审批", swapReq)
}

func RejectSwapRequest(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的申请ID")
		return
	}

	var req RespondSwapReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	var swapReq models.SwapRequest
	if err := config.DB.Preload("Requester").Preload("TargetUser").
		Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, uint(id)).Error; err != nil {
		utils.NotFound(c, "换班申请不存在")
		return
	}

	if swapReq.TargetUserID != userID {
		utils.Forbidden(c, "只有被申请人可以拒绝")
		return
	}

	if swapReq.Status != models.SwapStatusPending {
		utils.BadRequest(c, "当前状态不允许拒绝，状态为: "+string(swapReq.Status))
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":             models.SwapStatusRejected,
		"target_user_remark": req.Remark,
		"target_user_at":     &now,
	}

	if err := config.DB.Model(&swapReq).Updates(updates).Error; err != nil {
		utils.InternalServerError(c, "拒绝失败: "+err.Error())
		return
	}

	detail := fmt.Sprintf("拒绝换班申请，原因: %s", req.Remark)
	middleware.LogOperation(c, models.OpTypeRejectSwap, &swapReq.ID, nil, detail)

	reqShiftDesc := utils.BuildShiftDescription(&swapReq.RequesterShift)
	tgtShiftDesc := utils.BuildShiftDescription(&swapReq.TargetShift)
	meta := utils.SwapNotifMetadata{
		RequesterName:  swapReq.Requester.Name,
		TargetUserName: swapReq.TargetUser.Name,
		RequesterShift: reqShiftDesc,
		TargetShift:    tgtShiftDesc,
		Remark:         req.Remark,
	}
	notifyTitle := fmt.Sprintf("【换班被拒】%s 拒绝了你的换班申请", swapReq.TargetUser.Name)
	notifyContent := fmt.Sprintf("%s 拒绝了换班申请：\n  你的班次: %s\n  对方班次: %s\n  拒绝原因: %s\n\n  班次已重新开放申请，你可以重新发起换班申请。",
		swapReq.TargetUser.Name, reqShiftDesc, tgtShiftDesc, req.Remark)
	utils.NotifyUser(swapReq.RequesterID, models.NotifTypeSwapRejected, notifyTitle, notifyContent, &swapReq.ID, meta)

	utils.SuccessWithMessage(c, "已拒绝换班申请，班次已重新开放", swapReq)
}

func ApproveSwapRequest(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的申请ID")
		return
	}

	var req ApproveSwapReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	var approver models.User
	if err := config.DB.First(&approver, userID).Error; err != nil {
		utils.InternalServerError(c, "审批人信息获取失败")
		return
	}
	if approver.Role != models.RoleManager && approver.Role != models.RoleAdmin {
		utils.Forbidden(c, fmt.Sprintf("当前角色(%s)无审批权限，需要经理或管理员权限", approver.Role))
		return
	}

	var swapReq models.SwapRequest
	if err := config.DB.Preload("Requester").Preload("TargetUser").
		Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, uint(id)).Error; err != nil {
		utils.NotFound(c, "换班申请不存在")
		return
	}

	if swapReq.Status != models.SwapStatusAccepted {
		utils.BadRequest(c, "只有双方确认后的申请才能审批，当前状态: "+string(swapReq.Status))
		return
	}

	conflict, err := detectTimeSlotConflict(
		swapReq.RequesterID,
		swapReq.RequesterShift.ShiftDate,
		swapReq.RequesterShift.StartTime,
		swapReq.RequesterShift.EndTime,
		swapReq.ID,
	)
	if err != nil {
		utils.InternalServerError(c, "并发冲突检测失败")
		return
	}
	if conflict != nil {
		utils.BadRequest(c, fmt.Sprintf(
			"审批时发现申请人时段冲突(申请ID:%d)，请先处理冲突申请后再审批",
			conflict.ID,
		))
		return
	}
	conflict, err = detectTimeSlotConflict(
		swapReq.TargetUserID,
		swapReq.TargetShift.ShiftDate,
		swapReq.TargetShift.StartTime,
		swapReq.TargetShift.EndTime,
		swapReq.ID,
	)
	if err != nil {
		utils.InternalServerError(c, "并发冲突检测失败")
		return
	}
	if conflict != nil {
		utils.BadRequest(c, fmt.Sprintf(
			"审批时发现被申请人时段冲突(申请ID:%d)，请先处理冲突申请后再审批",
			conflict.ID,
		))
		return
	}

	tx := config.DB.Begin()

	now := time.Now()
	updates := map[string]interface{}{
		"status":           models.SwapStatusApproved,
		"approver_id":      &userID,
		"approval_remark":  req.Remark,
		"approved_at":      &now,
	}

	if err := tx.Model(&swapReq).Updates(updates).Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "审批失败: "+err.Error())
		return
	}

	if err := tx.Model(&swapReq.RequesterShift).Update("user_id", swapReq.TargetUserID).Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "更新申请人班次失败")
		return
	}

	if err := tx.Model(&swapReq.TargetShift).Update("user_id", swapReq.RequesterID).Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "更新目标班次失败")
		return
	}

	completeNow := time.Now()
	if err := tx.Model(&swapReq).Updates(map[string]interface{}{
		"status":       models.SwapStatusCompleted,
		"completed_at": &completeNow,
	}).Error; err != nil {
		tx.Rollback()
		utils.InternalServerError(c, "更新完成状态失败")
		return
	}

	tx.Commit()

	detail := fmt.Sprintf("[%s(%s)] 审批通过换班申请，备注: %s", approver.Name, approver.Role, req.Remark)
	middleware.LogOperation(c, models.OpTypeApproveSwap, &swapReq.ID, nil, detail)

	detail2 := "换班完成，双方班次已交换"
	middleware.LogOperation(c, models.OpTypeCompleteSwap, &swapReq.ID, nil, detail2)

	config.DB.Preload("Requester").Preload("TargetUser").Preload("Approver").
		Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, swapReq.ID)

	reqShiftDesc := utils.BuildShiftDescription(&swapReq.RequesterShift)
	tgtShiftDesc := utils.BuildShiftDescription(&swapReq.TargetShift)
	meta := utils.SwapNotifMetadata{
		RequesterName:  swapReq.Requester.Name,
		TargetUserName: swapReq.TargetUser.Name,
		ApproverName:   approver.Name,
		RequesterShift: reqShiftDesc,
		TargetShift:    tgtShiftDesc,
		Remark:         req.Remark,
	}

	for _, uid := range []uint{swapReq.RequesterID, swapReq.TargetUserID} {
		notifyTitle := "【换班完成】审批通过，换班已生效"
		notifyContent := fmt.Sprintf(
			"经理 %s 审批通过，换班已完成：\n  原班次已交换：\n    %s(%s) ←→ %s(%s)\n  审批备注: %s\n  请按新排班准时到岗。",
			approver.Name,
			swapReq.Requester.Name, reqShiftDesc,
			swapReq.TargetUser.Name, tgtShiftDesc,
			req.Remark,
		)
		utils.NotifyUser(uid, models.NotifTypeSwapCompleted, notifyTitle, notifyContent, &swapReq.ID, meta)
	}

	utils.SuccessWithMessage(c, "审批通过，换班已完成，双方均已收到通知", swapReq)
}

func DisapproveSwapRequest(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的申请ID")
		return
	}

	var req ApproveSwapReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	var approver models.User
	if err := config.DB.First(&approver, userID).Error; err != nil {
		utils.InternalServerError(c, "审批人信息获取失败")
		return
	}
	if approver.Role != models.RoleManager && approver.Role != models.RoleAdmin {
		utils.Forbidden(c, fmt.Sprintf("当前角色(%s)无审批权限，需要经理或管理员权限", approver.Role))
		return
	}

	var swapReq models.SwapRequest
	if err := config.DB.Preload("Requester").Preload("TargetUser").
		Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, uint(id)).Error; err != nil {
		utils.NotFound(c, "换班申请不存在")
		return
	}

	if swapReq.Status != models.SwapStatusAccepted {
		utils.BadRequest(c, "只有双方确认后的申请才能审批，当前状态: "+string(swapReq.Status))
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":          models.SwapStatusDisapproved,
		"approver_id":     &userID,
		"approval_remark": req.Remark,
		"approved_at":     &now,
	}

	if err := config.DB.Model(&swapReq).Updates(updates).Error; err != nil {
		utils.InternalServerError(c, "驳回失败: "+err.Error())
		return
	}

	detail := fmt.Sprintf("[%s(%s)] 驳回换班申请，原因: %s", approver.Name, approver.Role, req.Remark)
	middleware.LogOperation(c, models.OpTypeDisapproveSwap, &swapReq.ID, nil, detail)

	reqShiftDesc := utils.BuildShiftDescription(&swapReq.RequesterShift)
	tgtShiftDesc := utils.BuildShiftDescription(&swapReq.TargetShift)
	meta := utils.SwapNotifMetadata{
		RequesterName:  swapReq.Requester.Name,
		TargetUserName: swapReq.TargetUser.Name,
		ApproverName:   approver.Name,
		RequesterShift: reqShiftDesc,
		TargetShift:    tgtShiftDesc,
		Remark:         req.Remark,
	}

	for _, uid := range []uint{swapReq.RequesterID, swapReq.TargetUserID} {
		notifyTitle := fmt.Sprintf("【换班被驳回】经理 %s 驳回了换班申请", approver.Name)
		notifyContent := fmt.Sprintf(
			"经理 %s 驳回了换班申请：\n  申请班次：%s ←→ %s\n  驳回原因: %s\n\n  班次已回滚，可重新发起换班申请。",
			approver.Name, reqShiftDesc, tgtShiftDesc, req.Remark,
		)
		utils.NotifyUser(uid, models.NotifTypeSwapDisapproved, notifyTitle, notifyContent, &swapReq.ID, meta)
	}

	utils.SuccessWithMessage(c, "已驳回换班申请，班次已回滚，双方均已收到通知", swapReq)
}

func CancelSwapRequest(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的申请ID")
		return
	}

	var swapReq models.SwapRequest
	if err := config.DB.Preload("Requester").Preload("TargetUser").
		Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, uint(id)).Error; err != nil {
		utils.NotFound(c, "换班申请不存在")
		return
	}

	if swapReq.RequesterID != userID {
		utils.Forbidden(c, "只有申请人可以取消")
		return
	}

	if swapReq.Status != models.SwapStatusPending && swapReq.Status != models.SwapStatusAccepted {
		utils.BadRequest(c, "当前状态不允许取消，状态为: "+string(swapReq.Status))
		return
	}

	if err := config.DB.Model(&swapReq).Update("status", models.SwapStatusCanceled).Error; err != nil {
		utils.InternalServerError(c, "取消失败: "+err.Error())
		return
	}

	middleware.LogOperation(c, models.OpTypeCancelSwap, &swapReq.ID, nil, "取消换班申请")

	reqShiftDesc := utils.BuildShiftDescription(&swapReq.RequesterShift)
	tgtShiftDesc := utils.BuildShiftDescription(&swapReq.TargetShift)
	meta := utils.SwapNotifMetadata{
		RequesterName:  swapReq.Requester.Name,
		TargetUserName: swapReq.TargetUser.Name,
		RequesterShift: reqShiftDesc,
		TargetShift:    tgtShiftDesc,
	}

	notifyTitle := fmt.Sprintf("【换班取消】%s 取消了换班申请", swapReq.Requester.Name)
	notifyContent := fmt.Sprintf("%s 取消了换班申请：\n  你的班次: %s\n  对方班次: %s\n\n  班次已重新开放申请。",
		swapReq.Requester.Name, tgtShiftDesc, reqShiftDesc)
	utils.NotifyUser(swapReq.TargetUserID, models.NotifTypeSwapCanceled, notifyTitle, notifyContent, &swapReq.ID, meta)

	utils.SuccessWithMessage(c, "已取消换班申请，对方已收到通知，班次已重新开放", swapReq)
}
