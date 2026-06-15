package middleware

import (
	"fmt"
	"strings"

	"roster-swap-api/utils"

	"github.com/gin-gonic/gin"
)

type ContextKey string

const (
	UserIDKey   ContextKey = "user_id"
	UsernameKey ContextKey = "username"
	RoleKey     ContextKey = "role"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			utils.Unauthorized(c, "未提供认证令牌")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			utils.Unauthorized(c, "认证令牌格式错误")
			c.Abort()
			return
		}

		claims, err := utils.ParseToken(parts[1])
		if err != nil {
			utils.Unauthorized(c, "无效的认证令牌")
			c.Abort()
			return
		}

		c.Set(string(UserIDKey), claims.UserID)
		c.Set(string(UsernameKey), claims.Username)
		c.Set(string(RoleKey), claims.Role)

		c.Next()
	}
}

func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(string(RoleKey))
		if !exists {
			utils.Forbidden(c, "权限不足")
			c.Abort()
			return
		}

		roleStr := fmt.Sprintf("%v", role)
		if roleStr != "admin" {
			utils.Forbidden(c, "需要管理员权限")
			c.Abort()
			return
		}

		c.Next()
	}
}

func ManagerOrAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(string(RoleKey))
		if !exists {
			utils.Forbidden(c, "权限不足")
			c.Abort()
			return
		}

		roleStr := fmt.Sprintf("%v", role)
		if roleStr != "manager" && roleStr != "admin" {
			utils.Forbidden(c, "需要经理或管理员权限")
			c.Abort()
			return
		}

		c.Next()
	}
}

func GetCurrentUserID(c *gin.Context) uint {
	userID, exists := c.Get(string(UserIDKey))
	if !exists {
		return 0
	}
	return userID.(uint)
}

func GetCurrentUserRole(c *gin.Context) string {
	role, exists := c.Get(string(RoleKey))
	if !exists {
		return ""
	}
	return fmt.Sprintf("%v", role)
}
