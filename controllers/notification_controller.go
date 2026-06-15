package controllers

import (
	"roster-swap-api/config"
	"roster-swap-api/middleware"
	"roster-swap-api/models"
	"roster-swap-api/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func GetMyNotifications(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	status := c.Query("status")
	channel := c.Query("channel")
	notifType := c.Query("type")
	unreadOnly := c.Query("unread_only")

	query := config.DB.Model(&models.Notification{}).Preload("User").Where("user_id = ?", userID)

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if channel != "" {
		query = query.Where("channel = ?", channel)
	}
	if notifType != "" {
		query = query.Where("type = ?", notifType)
	}
	if unreadOnly == "true" {
		query = query.Where("read_at IS NULL")
	}

	var notifs []models.Notification
	if err := query.Order("created_at DESC").Find(&notifs).Error; err != nil {
		utils.InternalServerError(c, "获取通知列表失败")
		return
	}

	unreadCount := int64(0)
	config.DB.Model(&models.Notification{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Count(&unreadCount)

	utils.Success(c, gin.H{
		"items":       notifs,
		"total":       len(notifs),
		"unread_count": unreadCount,
	})
}

func GetNotification(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的通知ID")
		return
	}

	var notif models.Notification
	if err := config.DB.Preload("User").First(&notif, uint(id)).Error; err != nil {
		utils.NotFound(c, "通知不存在")
		return
	}

	if notif.UserID != userID {
		utils.Forbidden(c, "无权查看此通知")
		return
	}

	utils.Success(c, notif)
}

func MarkNotificationRead(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的通知ID")
		return
	}

	var notif models.Notification
	if err := config.DB.First(&notif, uint(id)).Error; err != nil {
		utils.NotFound(c, "通知不存在")
		return
	}

	if notif.UserID != userID {
		utils.Forbidden(c, "无权操作此通知")
		return
	}

	if notif.ReadAt == nil {
		now := time.Now()
		config.DB.Model(&notif).Updates(map[string]interface{}{
			"status":  models.NotifStatusRead,
			"read_at": &now,
		})
	}

	utils.SuccessWithMessage(c, "已标记为已读", notif)
}

func MarkAllNotificationsRead(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	now := time.Now()
	result := config.DB.Model(&models.Notification{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Updates(map[string]interface{}{
			"status":  models.NotifStatusRead,
			"read_at": &now,
		})

	if result.Error != nil {
		utils.InternalServerError(c, "批量标记失败: "+result.Error.Error())
		return
	}

	utils.SuccessWithMessage(c, "已全部标记为已读", gin.H{
		"affected_count": result.RowsAffected,
	})
}
