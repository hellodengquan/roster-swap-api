package controllers

import (
	"fmt"
	"time"

	"roster-swap-api/config"
	"roster-swap-api/middleware"
	"roster-swap-api/models"
	"roster-swap-api/utils"

	"github.com/gin-gonic/gin"
)

type CreateDelegateRequest struct {
	DelegateID uint   `json:"delegate_id" binding:"required"`
	StartDate  string `json:"start_date" binding:"required"`
	EndDate    string `json:"end_date" binding:"required"`
	Reason     string `json:"reason" binding:"required,max=500"`
}

type RevokeDelegateRequest struct {
	Reason string `json:"reason"`
}

func GetActiveDelegate(delegatorID uint) (*models.ApproverDelegate, error) {
	now := time.Now()
	var delegate models.ApproverDelegate
	err := config.DB.Where(
		"delegator_id = ? AND status = ? AND start_date <= ? AND end_date >= ?",
		delegatorID, models.DelegateStatusActive, now, now,
	).Preload("Delegator").Preload("Delegate").First(&delegate).Error
	if err != nil {
		return nil, err
	}
	return &delegate, nil
}

func CreateDelegate(c *gin.Context) {
	delegatorID := middleware.GetCurrentUserID(c)

	var delegator models.User
	if err := config.DB.First(&delegator, delegatorID).Error; err != nil {
		utils.InternalServerError(c, "获取用户信息失败")
		return
	}
	if delegator.Role != models.RoleManager && delegator.Role != models.RoleAdmin {
		utils.Forbidden(c, "只有经理或管理员可以设置审批代理")
		return
	}

	var req CreateDelegateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	if req.DelegateID == delegatorID {
		utils.BadRequest(c, "不能代理给自己")
		return
	}

	var delegateUser models.User
	if err := config.DB.First(&delegateUser, req.DelegateID).Error; err != nil {
		utils.NotFound(c, "代理用户不存在")
		return
	}
	if delegateUser.Role != models.RoleManager && delegateUser.Role != models.RoleAdmin {
		utils.BadRequest(c, "被代理人必须也是经理或管理员")
		return
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		utils.BadRequest(c, "开始日期格式错误，请使用 YYYY-MM-DD")
		return
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		utils.BadRequest(c, "结束日期格式错误，请使用 YYYY-MM-DD")
		return
	}

	endDate = endDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	if endDate.Before(startDate) {
		utils.BadRequest(c, "结束日期不能早于开始日期")
		return
	}

	now := time.Now()
	var existingActive models.ApproverDelegate
	err = config.DB.Where(
		"delegator_id = ? AND status = ? AND end_date >= ?",
		delegatorID, models.DelegateStatusActive, now,
	).First(&existingActive).Error
	if err == nil {
		utils.BadRequest(c, fmt.Sprintf("已存在有效代理(ID:%d)，请先撤销后再设置新代理", existingActive.ID))
		return
	}

	var existingOverlap models.ApproverDelegate
	err = config.DB.Where(
		"delegator_id = ? AND status = ? AND start_date <= ? AND end_date >= ?",
		delegatorID, models.DelegateStatusActive, endDate, startDate,
	).First(&existingOverlap).Error
	if err == nil {
		utils.BadRequest(c, fmt.Sprintf("存在重叠的有效代理(ID:%d)", existingOverlap.ID))
		return
	}

	delegate := &models.ApproverDelegate{
		DelegatorID: delegatorID,
		DelegateID:  req.DelegateID,
		StartDate:   startDate,
		EndDate:     endDate,
		Reason:      req.Reason,
		Status:      models.DelegateStatusActive,
	}

	if err := config.DB.Create(delegate).Error; err != nil {
		utils.InternalServerError(c, "创建代理失败: "+err.Error())
		return
	}

	config.DB.Preload("Delegator").Preload("Delegate").First(delegate, delegate.ID)

	detail := fmt.Sprintf("设置审批代理: %s → %s，有效期 %s ~ %s，原因: %s",
		delegator.Name, delegateUser.Name, req.StartDate, req.EndDate, req.Reason)
	middleware.LogOperation(c, "create_delegate", nil, nil, detail)

	for _, uid := range []uint{delegatorID, req.DelegateID} {
		title := "【审批代理设置】"
		content := fmt.Sprintf(
			"%s 已将审批权代理给 %s，\n有效期: %s 至 %s\n原因: %s",
			delegator.Name, delegateUser.Name, req.StartDate, req.EndDate, req.Reason)
		utils.NotifyUser(uid, models.NotifTypeSwapCreated, title, content, nil, nil)
	}

	utils.SuccessWithMessage(c, "审批代理已设置", delegate)
}

func RevokeDelegate(c *gin.Context) {
	delegatorID := middleware.GetCurrentUserID(c)

	delegateID, err := parseUintParam(c, "id")
	if err != nil {
		utils.BadRequest(c, "无效的代理ID")
		return
	}

	var delegate models.ApproverDelegate
	if err := config.DB.First(&delegate, delegateID).Error; err != nil {
		utils.NotFound(c, "代理记录不存在")
		return
	}

	if delegate.DelegatorID != delegatorID {
		role := middleware.GetCurrentUserRole(c)
		if role != "admin" {
			utils.Forbidden(c, "只能撤销自己设置的代理")
			return
		}
	}

	if delegate.Status != models.DelegateStatusActive {
		utils.BadRequest(c, fmt.Sprintf("当前代理状态为 %s，无需撤销", delegate.Status))
		return
	}

	var req RevokeDelegateRequest
	c.ShouldBindJSON(&req)

	now := time.Now()
	delegate.Status = models.DelegateStatusRevoked
	delegate.EndDate = now

	if err := config.DB.Save(&delegate).Error; err != nil {
		utils.InternalServerError(c, "撤销代理失败: "+err.Error())
		return
	}

	config.DB.Preload("Delegator").Preload("Delegate").First(&delegate, delegateID)

	detail := fmt.Sprintf("撤销审批代理: %s → %s，撤销原因: %s",
		delegate.Delegator.Name, delegate.Delegate.Name, req.Reason)
	middleware.LogOperation(c, "revoke_delegate", nil, nil, detail)

	for _, uid := range []uint{delegate.DelegatorID, delegate.DelegateID} {
		title := "【审批代理撤销】"
		content := fmt.Sprintf(
			"%s 的审批代理已撤销。\n原代理人: %s\n撤销原因: %s",
			delegate.Delegator.Name, delegate.Delegate.Name, req.Reason)
		utils.NotifyUser(uid, models.NotifTypeSwapCreated, title, content, nil, nil)
	}

	utils.SuccessWithMessage(c, "代理已撤销", delegate)
}

func GetMyDelegates(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	var delegates []models.ApproverDelegate
	err := config.DB.Where("delegator_id = ? OR delegate_id = ?", userID, userID).
		Preload("Delegator").Preload("Delegate").
		Order("created_at DESC").
		Find(&delegates).Error
	if err != nil {
		utils.InternalServerError(c, "查询代理列表失败")
		return
	}

	utils.Success(c, delegates)
}

func GetActiveDelegateInfo(c *gin.Context) {
	delegatorID, err := parseUintParam(c, "delegator_id")
	if err != nil {
		userID := middleware.GetCurrentUserID(c)
		delegatorID = userID
	}

	delegate, err := GetActiveDelegate(delegatorID)
	if err != nil {
		utils.Success(c, gin.H{"has_active_delegate": false})
		return
	}

	utils.Success(c, gin.H{
		"has_active_delegate": true,
		"delegate":            delegate,
	})
}

func CheckApprovalPermission(userID uint) (*models.User, *models.ApproverDelegate, error) {
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		return nil, nil, fmt.Errorf("用户不存在")
	}

	if user.Role == models.RoleManager || user.Role == models.RoleAdmin {
		return &user, nil, nil
	}

	delegate, err := GetActiveDelegateForUser(userID)
	if err == nil && delegate.IsActive() {
		var delegator models.User
		config.DB.First(&delegator, delegate.DelegatorID)
		return &delegator, delegate, nil
	}

	return nil, nil, fmt.Errorf("当前用户无审批权限，也不是有效的审批代理人")
}

func GetActiveDelegateForUser(delegateUserID uint) (*models.ApproverDelegate, error) {
	now := time.Now()
	var delegate models.ApproverDelegate
	err := config.DB.Where(
		"delegate_id = ? AND status = ? AND start_date <= ? AND end_date >= ?",
		delegateUserID, models.DelegateStatusActive, now, now,
	).Preload("Delegator").Preload("Delegate").First(&delegate).Error
	if err != nil {
		return nil, err
	}
	return &delegate, nil
}

func parseUintParam(c *gin.Context, param string) (uint, error) {
	idStr := c.Param(param)
	if idStr == "" {
		idStr = c.Query(param)
	}
	id, err := parseUint64Param(idStr)
	return uint(id), err
}

func parseUint64Param(s string) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	var result uint64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid char: %c", ch)
		}
		result = result*10 + uint64(ch-'0')
	}
	return result, nil
}
