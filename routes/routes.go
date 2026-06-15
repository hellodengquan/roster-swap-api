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
		}

		logs := api.Group("/logs")
		logs.Use(middleware.AuthMiddleware())
		{
			logs.GET("/my", controllers.GetMyOperationLogs)
			logs.GET("", middleware.ManagerOrAdmin(), controllers.GetOperationLogs)
		}
	}
}
