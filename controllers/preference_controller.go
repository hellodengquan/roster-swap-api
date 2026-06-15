package controllers

import (
	"roster-swap-api/config"
	"roster-swap-api/middleware"
	"roster-swap-api/models"
	"roster-swap-api/utils"

	"github.com/gin-gonic/gin"
)

type UpdatePreferenceRequest struct {
	EnabledChannels        []models.NotificationChannel `json:"enabled_channels"`
	SwapRequestNotify    *bool `json:"swap_request_notify,omitempty"`
	ApprovalNotify       *bool `json:"approval_notify,omitempty"`
	EmailNotify      *bool `json:"email_notify,omitempty"`
	IMNotify         *bool `json:"im_notify,omitempty"`
	InAppNotify      *bool `json:"inapp_notify,omitempty"`
	QuietHours       string `json:"quiet_hours,omitempty"`
}

type DispatchNotificationRequest struct {
	UserID           uint                      `json:"user_id" binding:"required"`
	Title            string                    `json:"title" binding:"required"`
	Content          string                    `json:"content" binding:"required"`
	Channels         []models.NotificationChannel `json:"channels"`
}

func GetMyPreferences(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)
	pref := utils.GetUserPreference(userID)

	utils.Success(c, gin.H{
		"id":                        pref.ID,
		"user_id":                   pref.UserID,
		"enabled_channels":          pref.GetNotifyChannels(),
		"swap_request_notify_enabled": pref.SwapRequestNotifyEnabled,
		"approval_notify_enabled":    pref.ApprovalNotifyEnabled,
		"email_notify_enabled":     pref.EmailNotifyEnabled,
		"im_notify_enabled":    pref.IMNotifyEnabled,
		"inapp_notify_enabled": pref.InAppNotifyEnabled,
		"quiet_hours":           pref.NotificationQuietHours,
	})
}

func UpdatePreferences(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	var req UpdatePreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	pref := utils.GetUserPreference(userID)

	if len(req.EnabledChannels) > 0 {
		pref.SetNotifyChannels(req.EnabledChannels)
	}

	if req.SwapRequestNotify != nil {
		pref.SwapRequestNotifyEnabled = *req.SwapRequestNotify
	}
	if req.ApprovalNotify != nil {
		pref.ApprovalNotifyEnabled = *req.ApprovalNotify
	}
	if req.EmailNotify != nil {
		pref.EmailNotifyEnabled = *req.EmailNotify
	}
	if req.IMNotify != nil {
		pref.IMNotifyEnabled = *req.IMNotify
	}
	if req.InAppNotify != nil {
		pref.InAppNotifyEnabled = *req.InAppNotify
	}
	if req.QuietHours != "" {
		pref.NotificationQuietHours = req.QuietHours
	}

	if err := utils.SaveUserPreference(pref); err != nil {
		utils.InternalServerError(c, "保存偏好失败: "+err.Error())
		return
	}

	utils.SuccessWithMessage(c, "偏好设置已更新", gin.H{
		"enabled_channels": pref.GetNotifyChannels(),
		"swap_request_notify_enabled": pref.SwapRequestNotifyEnabled,
		"approval_notify_enabled": pref.ApprovalNotifyEnabled,
	})
}

func DispatchNotification(c *gin.Context) {
	role := middleware.GetCurrentUserRole(c)
	if role != "manager" && role != "admin" {
		utils.Forbidden(c, "只有经理或管理员可以派发通知")
		return
	}

	var req DispatchNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	var user models.User
	if err := config.DB.First(&user, req.UserID).Error; err != nil {
		utils.NotFound(c, "目标用户不存在")
		return
	}

	if len(req.Channels) == 0 {
		req.Channels = []models.NotificationChannel{models.ChannelInApp}
	}

	utils.NotifyUser(req.UserID, models.NotifTypeSwapCreated, req.Title, req.Content, nil, nil, req.Channels...)

	utils.SuccessWithMessage(c, "通知已派发", gin.H{
		"channels_used": req.Channels,
		"target_user": user.Name,
	})
}
