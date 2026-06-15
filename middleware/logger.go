package middleware

import (
	"roster-swap-api/config"
	"roster-swap-api/models"

	"github.com/gin-gonic/gin"
)

func LogOperation(c *gin.Context, opType models.OperationType, swapID *uint, shiftID *uint, detail string) {
	userID, _ := c.Get(string(UserIDKey))
	ip := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	log := &models.OperationLog{
		UserID:        userID.(uint),
		OperationType: opType,
		SwapRequestID: swapID,
		ShiftID:       shiftID,
		Detail:        detail,
		IPAddress:     ip,
		UserAgent:     userAgent,
	}

	config.DB.Create(log)
}
