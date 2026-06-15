package controllers

import (
	"roster-swap-api/config"
	"roster-swap-api/middleware"
	"roster-swap-api/models"
	"roster-swap-api/utils"

	"github.com/gin-gonic/gin"
)

func GetOperationLogs(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)
	role := middleware.GetCurrentUserRole(c)

	swapID := c.Query("swap_id")
	shiftID := c.Query("shift_id")
	opType := c.Query("type")
	targetUserID := c.Query("user_id")

	query := config.DB.Model(&models.OperationLog{}).Preload("User")

	if role == "employee" {
		query = query.Where("user_id = ?", userID)
	} else if targetUserID != "" {
		query = query.Where("user_id = ?", targetUserID)
	}

	if swapID != "" {
		query = query.Where("swap_request_id = ?", swapID)
	}
	if shiftID != "" {
		query = query.Where("shift_id = ?", shiftID)
	}
	if opType != "" {
		query = query.Where("operation_type = ?", opType)
	}

	var logs []models.OperationLog
	if err := query.Order("created_at DESC").Find(&logs).Error; err != nil {
		utils.InternalServerError(c, "获取操作日志失败")
		return
	}

	utils.Success(c, logs)
}

func GetMyOperationLogs(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	swapID := c.Query("swap_id")
	shiftID := c.Query("shift_id")
	opType := c.Query("type")

	query := config.DB.Model(&models.OperationLog{}).Preload("User").Where("user_id = ?", userID)

	if swapID != "" {
		query = query.Where("swap_request_id = ?", swapID)
	}
	if shiftID != "" {
		query = query.Where("shift_id = ?", shiftID)
	}
	if opType != "" {
		query = query.Where("operation_type = ?", opType)
	}

	var logs []models.OperationLog
	if err := query.Order("created_at DESC").Find(&logs).Error; err != nil {
		utils.InternalServerError(c, "获取我的操作日志失败")
		return
	}

	utils.Success(c, logs)
}
