package routes

import (
	"roster-swap-api/controllers"
	"roster-swap-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/register", controllers.Register)
			auth.POST("/login", controllers.Login)
			auth.GET("/me", middleware.AuthMiddleware(), controllers.GetCurrentUser)
			auth.GET("/users", middleware.AuthMiddleware(), middleware.ManagerOrAdmin(), controllers.GetUserList)
		}

		shifts := api.Group("/shifts")
		shifts.Use(middleware.AuthMiddleware())
		{
			shifts.GET("/my", controllers.GetMyShifts)
			shifts.GET("/:id", controllers.GetShift)
			shifts.GET("", controllers.GetShiftList)
			shifts.POST("", middleware.ManagerOrAdmin(), controllers.CreateShift)
			shifts.PUT("/:id", middleware.ManagerOrAdmin(), controllers.UpdateShift)
			shifts.DELETE("/:id", middleware.ManagerOrAdmin(), controllers.DeleteShift)
		}

		swaps := api.Group("/swaps")
		swaps.Use(middleware.AuthMiddleware())
		{
			swaps.GET("", controllers.GetSwapRequestList)
			swaps.GET("/:id", controllers.GetSwapRequest)
			swaps.POST("", controllers.CreateSwapRequest)
			swaps.POST("/:id/accept", controllers.AcceptSwapRequest)
			swaps.POST("/:id/reject", controllers.RejectSwapRequest)
			swaps.POST("/:id/approve", middleware.ManagerOrAdmin(), controllers.ApproveSwapRequest)
			swaps.POST("/:id/disapprove", middleware.ManagerOrAdmin(), controllers.DisapproveSwapRequest)
			swaps.POST("/:id/cancel", controllers.CancelSwapRequest)
			swaps.POST("/batch/approve", middleware.ManagerOrAdmin(), controllers.BatchApproveSwap)
			swaps.POST("/batch/disapprove", middleware.ManagerOrAdmin(), controllers.BatchDisapproveSwap)
		}

		logs := api.Group("/logs")
		logs.Use(middleware.AuthMiddleware())
		{
			logs.GET("/my", controllers.GetMyOperationLogs)
			logs.GET("", middleware.ManagerOrAdmin(), controllers.GetOperationLogs)
			logs.GET("/export", middleware.ManagerOrAdmin(), controllers.ExportOperationLogs)
		}

		notifs := api.Group("/notifications")
		notifs.Use(middleware.AuthMiddleware())
		{
			notifs.GET("", controllers.GetMyNotifications)
			notifs.GET("/:id", controllers.GetNotification)
			notifs.POST("/:id/read", controllers.MarkNotificationRead)
			notifs.POST("/read-all", controllers.MarkAllNotificationsRead)
			notifs.POST("/dispatch", middleware.ManagerOrAdmin(), controllers.DispatchNotification)
		}

		prefs := api.Group("/preferences")
		prefs.Use(middleware.AuthMiddleware())
		{
			prefs.GET("", controllers.GetMyPreferences)
			prefs.PUT("", controllers.UpdatePreferences)
		}

		delegates := api.Group("/delegates")
		delegates.Use(middleware.AuthMiddleware())
		{
			delegates.GET("", controllers.GetMyDelegates)
			delegates.GET("/active", controllers.GetActiveDelegateInfo)
			delegates.POST("", middleware.ManagerOrAdmin(), controllers.CreateDelegate)
			delegates.POST("/:id/revoke", middleware.ManagerOrAdmin(), controllers.RevokeDelegate)
		}
	}
}
