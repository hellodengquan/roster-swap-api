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

	var pendingSwap models.SwapRequest
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
	err := config.DB.Where(sql, params...).First(&pendingSwap).Error
	if err == nil {
		utils.BadRequest(c, "该班次已有进行中的换班申请")
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
		requesterShift.ShiftDate.Format("2006-01-02")+" "+requesterShift.StartTime+"-"+requesterShift.EndTime,
		targetShift.ShiftDate.Format("2006-01-02")+" "+targetShift.StartTime+"-"+targetShift.EndTime,
		req.Reason)
	middleware.LogOperation(c, models.OpTypeCreateSwap, &swapReq.ID, nil, detail)

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
	if err := config.DB.Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, uint(id)).Error; err != nil {
		utils.NotFound(c, "换班申请不存在")
		return
	}

	if swapReq.TargetUserID != userID {
		utils.Forbidden(c, "只有被申请人可以确认")
		return
	}

	if swapReq.Status != models.SwapStatusPending {
		utils.BadRequest(c, "当前状态不允许确认")
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

	config.DB.Preload("Requester").Preload("TargetUser").
		Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, swapReq.ID)

	utils.SuccessWithMessage(c, "已确认换班申请", swapReq)
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
	if err := config.DB.First(&swapReq, uint(id)).Error; err != nil {
		utils.NotFound(c, "换班申请不存在")
		return
	}

	if swapReq.TargetUserID != userID {
		utils.Forbidden(c, "只有被申请人可以拒绝")
		return
	}

	if swapReq.Status != models.SwapStatusPending {
		utils.BadRequest(c, "当前状态不允许拒绝")
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

	utils.SuccessWithMessage(c, "已拒绝换班申请", swapReq)
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

	var swapReq models.SwapRequest
	if err := config.DB.Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, uint(id)).Error; err != nil {
		utils.NotFound(c, "换班申请不存在")
		return
	}

	if swapReq.Status != models.SwapStatusAccepted {
		utils.BadRequest(c, "只有双方确认后的申请才能审批")
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

	detail := fmt.Sprintf("审批通过换班申请，备注: %s", req.Remark)
	middleware.LogOperation(c, models.OpTypeApproveSwap, &swapReq.ID, nil, detail)

	detail2 := "换班完成，双方班次已交换"
	middleware.LogOperation(c, models.OpTypeCompleteSwap, &swapReq.ID, nil, detail2)

	config.DB.Preload("Requester").Preload("TargetUser").Preload("Approver").
		Preload("RequesterShift").Preload("TargetShift").
		First(&swapReq, swapReq.ID)

	utils.SuccessWithMessage(c, "审批通过，换班已完成", swapReq)
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

	var swapReq models.SwapRequest
	if err := config.DB.First(&swapReq, uint(id)).Error; err != nil {
		utils.NotFound(c, "换班申请不存在")
		return
	}

	if swapReq.Status != models.SwapStatusAccepted {
		utils.BadRequest(c, "只有双方确认后的申请才能审批")
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

	detail := fmt.Sprintf("驳回换班申请，原因: %s", req.Remark)
	middleware.LogOperation(c, models.OpTypeDisapproveSwap, &swapReq.ID, nil, detail)

	utils.SuccessWithMessage(c, "已驳回换班申请", swapReq)
}

func CancelSwapRequest(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的申请ID")
		return
	}

	var swapReq models.SwapRequest
	if err := config.DB.First(&swapReq, uint(id)).Error; err != nil {
		utils.NotFound(c, "换班申请不存在")
		return
	}

	if swapReq.RequesterID != userID {
		utils.Forbidden(c, "只有申请人可以取消")
		return
	}

	if swapReq.Status != models.SwapStatusPending && swapReq.Status != models.SwapStatusAccepted {
		utils.BadRequest(c, "当前状态不允许取消")
		return
	}

	if err := config.DB.Model(&swapReq).Update("status", models.SwapStatusCanceled).Error; err != nil {
		utils.InternalServerError(c, "取消失败: "+err.Error())
		return
	}

	middleware.LogOperation(c, models.OpTypeCancelSwap, &swapReq.ID, nil, "取消换班申请")

	utils.SuccessWithMessage(c, "已取消换班申请", swapReq)
}
