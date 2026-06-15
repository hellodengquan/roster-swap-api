package main

import (
	"fmt"
	"log"

	"roster-swap-api/config"
	"roster-swap-api/routes"
	"roster-swap-api/seed"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := config.LoadConfig(); err != nil {
		log.Fatal("加载配置失败:", err)
	}

	db, err := config.InitDB()
	if err != nil {
		log.Fatal("数据库初始化失败:", err)
	}
	fmt.Println("数据库连接成功")

	sqlDB, err := db.DB()
	if err == nil {
		defer sqlDB.Close()
	}

	if err := seed.SeedData(db); err != nil {
		log.Println("种子数据初始化警告:", err)
	}

	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	routes.SetupRoutes(r)

	fmt.Println("")
	fmt.Println("========================================")
	fmt.Println("  换班申请系统 API 服务启动成功")
	fmt.Println("  服务地址: http://localhost:" + config.AppConfig.Port)
	fmt.Println("========================================")
	fmt.Println("")

	if err := r.Run(":" + config.AppConfig.Port); err != nil {
		log.Fatal("服务启动失败:", err)
	}
}
