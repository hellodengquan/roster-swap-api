package controllers

import (
	"strconv"
	"time"

	"roster-swap-api/config"
	"roster-swap-api/middleware"
	"roster-swap-api/models"
	"roster-swap-api/utils"

	"github.com/gin-gonic/gin"
)

type CreateShiftRequest struct {
	UserID    uint   `json:"user_id" binding:"required"`
	ShiftDate string `json:"shift_date" binding:"required"`
	StartTime string `json:"start_time" binding:"required"`
	EndTime   string `json:"end_time" binding:"required"`
	ShiftType string `json:"shift_type"`
	Location  string `json:"location"`
	Note      string `json:"note"`
	Timezone  string `json:"timezone"`
}

type UpdateShiftRequest struct {
	ShiftDate string `json:"shift_date"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	ShiftType string `json:"shift_type"`
	Status    string `json:"status"`
	Location  string `json:"location"`
	Note      string `json:"note"`
}

func CreateShift(c *gin.Context) {
	var req CreateShiftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	shiftDate, err := time.Parse("2006-01-02", req.ShiftDate)
	if err != nil {
		utils.BadRequest(c, "日期格式错误，请使用 YYYY-MM-DD")
		return
	}

	var user models.User
	if err := config.DB.First(&user, req.UserID).Error; err != nil {
		utils.NotFound(c, "用户不存在")
		return
	}

	timezone := user.Timezone
	if req.Timezone != "" {
		timezone = req.Timezone
	}

	startUTC, endUTC, err := utils.ComputeShiftUTCTimes(shiftDate, req.StartTime, req.EndTime, timezone)
	if err != nil {
		utils.BadRequest(c, "时区时间转换失败: "+err.Error())
		return
	}

	shift := &models.Shift{
		UserID:       req.UserID,
		ShiftDate:    shiftDate,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		StartTimeUTC: startUTC,
		EndTimeUTC:   endUTC,
		Timezone:     timezone,
		ShiftType:    req.ShiftType,
		Status:       models.ShiftStatusActive,
		Location:     req.Location,
		Note:         req.Note,
	}

	if err := config.DB.Create(shift).Error; err != nil {
		utils.InternalServerError(c, "创建班次失败: "+err.Error())
		return
	}

	shift.User = user

	middleware.LogOperation(c, models.OpTypeCreateShift, nil, &shift.ID, "创建班次: "+req.ShiftDate+" "+req.StartTime+"-"+req.EndTime)

	utils.Success(c, shift)
}

func GetShiftList(c *gin.Context) {
	userID := c.Query("user_id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	status := c.Query("status")

	query := config.DB.Model(&models.Shift{}).Preload("User")

	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if startDate != "" {
		query = query.Where("shift_date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("shift_date <= ?", endDate)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var shifts []models.Shift
	if err := query.Order("shift_date DESC, start_time ASC").Find(&shifts).Error; err != nil {
		utils.InternalServerError(c, "获取班次列表失败")
		return
	}

	utils.Success(c, shifts)
}

func GetShift(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的班次ID")
		return
	}

	var shift models.Shift
	if err := config.DB.Preload("User").First(&shift, uint(id)).Error; err != nil {
		utils.NotFound(c, "班次不存在")
		return
	}

	utils.Success(c, shift)
}

func UpdateShift(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的班次ID")
		return
	}

	var req UpdateShiftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	var shift models.Shift
	if err := config.DB.First(&shift, uint(id)).Error; err != nil {
		utils.NotFound(c, "班次不存在")
		return
	}

	updates := make(map[string]interface{})
	needUTCUpdate := false
	newShiftDate := shift.ShiftDate
	newStartTime := shift.StartTime
	newEndTime := shift.EndTime
	newTimezone := shift.Timezone

	if req.ShiftDate != "" {
		parsed, err := time.Parse("2006-01-02", req.ShiftDate)
		if err != nil {
			utils.BadRequest(c, "日期格式错误，请使用 YYYY-MM-DD")
			return
		}
		newShiftDate = parsed
		updates["shift_date"] = parsed
		needUTCUpdate = true
	}
	if req.StartTime != "" {
		newStartTime = req.StartTime
		updates["start_time"] = req.StartTime
		needUTCUpdate = true
	}
	if req.EndTime != "" {
		newEndTime = req.EndTime
		updates["end_time"] = req.EndTime
		needUTCUpdate = true
	}

	if needUTCUpdate {
		var user models.User
		config.DB.First(&user, shift.UserID)
		timezone := user.Timezone
		if newTimezone == "" {
			newTimezone = timezone
		}

		startUTC, endUTC, err := utils.ComputeShiftUTCTimes(newShiftDate, newStartTime, newEndTime, newTimezone)
		if err != nil {
			utils.BadRequest(c, "时区时间转换失败: "+err.Error())
			return
		}
		updates["start_time_utc"] = startUTC
		updates["end_time_utc"] = endUTC
		updates["timezone"] = newTimezone
	}
	if req.ShiftType != "" {
		updates["shift_type"] = req.ShiftType
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.Location != "" {
		updates["location"] = req.Location
	}
	if req.Note != "" {
		updates["note"] = req.Note
	}

	if err := config.DB.Model(&shift).Updates(updates).Error; err != nil {
		utils.InternalServerError(c, "更新班次失败: "+err.Error())
		return
	}

	middleware.LogOperation(c, models.OpTypeUpdateShift, nil, &shift.ID, "更新班次信息")

	utils.Success(c, shift)
}

func DeleteShift(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		utils.BadRequest(c, "无效的班次ID")
		return
	}

	var shift models.Shift
	if err := config.DB.First(&shift, uint(id)).Error; err != nil {
		utils.NotFound(c, "班次不存在")
		return
	}

	if err := config.DB.Delete(&shift).Error; err != nil {
		utils.InternalServerError(c, "删除班次失败: "+err.Error())
		return
	}

	middleware.LogOperation(c, models.OpTypeDeleteShift, nil, &shift.ID, "删除班次")

	utils.SuccessWithMessage(c, "删除成功", nil)
}

func GetMyShifts(c *gin.Context) {
	userID := middleware.GetCurrentUserID(c)

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	status := c.Query("status")

	query := config.DB.Model(&models.Shift{}).Preload("User").Where("user_id = ?", userID)

	if startDate != "" {
		query = query.Where("shift_date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("shift_date <= ?", endDate)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var shifts []models.Shift
	if err := query.Order("shift_date DESC, start_time ASC").Find(&shifts).Error; err != nil {
		utils.InternalServerError(c, "获取我的班次失败")
		return
	}

	utils.Success(c, shifts)
}
