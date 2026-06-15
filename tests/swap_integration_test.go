package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"roster-swap-api/models"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAuthRoutes(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("Login with correct credentials", func(t *testing.T) {
		body := gin.H{"username": "emp1", "password": "123456"}
		resp := tc.MakeRequest(t, "POST", "/api/auth/login", "", body, http.StatusOK)
		assert.Equal(t, 0, resp.Code)
		assert.NotNil(t, resp.Data)
	})

	t.Run("Login with wrong password", func(t *testing.T) {
		body := gin.H{"username": "emp1", "password": "wrong"}
		resp := tc.MakeRequest(t, "POST", "/api/auth/login", "", body, http.StatusBadRequest)
		assert.NotEqual(t, 0, resp.Code)
	})

	t.Run("Get current user with valid token", func(t *testing.T) {
		resp := tc.MakeRequest(t, "GET", "/api/auth/me", tc.Tokens["emp1"], nil, http.StatusOK)
		assert.Equal(t, 0, resp.Code)
	})

	t.Run("Protected route without token", func(t *testing.T) {
		tc.MakeRequest(t, "GET", "/api/auth/me", "", nil, http.StatusUnauthorized)
	})
}

func TestFullSwapFlow(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("完整换班流程", func(t *testing.T) {
		swapID, _ := tc.CreateSwap(t, "emp1", "emp2", 0, 0, "有事换班", http.StatusOK)
		tc.AssertSwapStatus(t, swapID, models.SwapStatusPending)

		tc.AssertNotificationCount(t, "emp1", 3, models.NotifTypeSwapCreated)
		tc.AssertNotificationCount(t, "emp2", 3, models.NotifTypeSwapCreated)

		tc.MakeRequest(t, "POST",
		fmt.Sprintf("/api/swaps/%d/accept", swapID),
		tc.Tokens["emp2"],
		gin.H{"remark": "好的"},
		http.StatusOK,
	)
		tc.AssertSwapStatus(t, swapID, models.SwapStatusAccepted)
		tc.AssertNotificationCount(t, "emp1", 3, models.NotifTypeSwapAccepted)
		tc.AssertOperationLogs(t, models.OpTypeAcceptSwap, 1)

		reqShiftBefore := tc.ShiftIDs["emp1"][0]
		tgtShiftBefore := tc.ShiftIDs["emp2"][0]
		tc.AssertShiftOwner(t, reqShiftBefore, "emp1")
		tc.AssertShiftOwner(t, tgtShiftBefore, "emp2")

		tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/approve", swapID),
			tc.Tokens["manager"],
			gin.H{"remark": "同意"},
			http.StatusOK,
		)
		tc.AssertSwapStatus(t, swapID, models.SwapStatusCompleted)
		tc.AssertShiftOwner(t, reqShiftBefore, "emp2")
		tc.AssertShiftOwner(t, tgtShiftBefore, "emp1")

		tc.AssertNotificationCount(t, "emp1", 3, models.NotifTypeSwapCompleted)
		tc.AssertNotificationCount(t, "emp2", 3, models.NotifTypeSwapCompleted)
		tc.AssertOperationLogs(t, models.OpTypeCompleteSwap, 1)
		tc.AssertOperationLogs(t, models.OpTypeApproveSwap, 1)
	})
}

func TestRejectRollback(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("被拒回滚-申请可重新发起", func(t *testing.T) {
		swapID1, _ := tc.CreateSwap(t, "emp1", "emp2", 0, 0, "换班A", http.StatusOK)
		tc.AssertSwapStatus(t, swapID1, models.SwapStatusPending)

		tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/reject", swapID1),
			tc.Tokens["emp2"],
			gin.H{"remark": "那天我也有事"},
			http.StatusOK,
		)
		tc.AssertSwapStatus(t, swapID1, models.SwapStatusRejected)
		tc.AssertNotificationCount(t, "emp1", 3, models.NotifTypeSwapRejected)
		tc.AssertOperationLogs(t, models.OpTypeRejectSwap, 1)

		swapID2, resp := tc.CreateSwap(t, "emp1", "emp2", 0, 0, "换班B-重新申请", http.StatusOK)
		assert.Equal(t, 0, resp.Code, "被拒后应能重新发起申请")
		assert.NotZero(t, swapID2, "应能创建新申请")
	})
}

func TestConcurrentConflict(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("并发时段冲突检测", func(t *testing.T) {
		swapID, _ := tc.CreateSwap(t, "emp1", "emp2", 0, 0, "申请1", http.StatusOK)
		tc.AssertSwapStatus(t, swapID, models.SwapStatusPending)

		_, resp := tc.CreateSwap(t, "emp1", "emp3", 0, 0, "申请2-同日时段冲突", http.StatusBadRequest)
		assert.NotEqual(t, 0, resp.Code, "同日同时段应被拒绝")
		assert.True(t,
			len(resp.Message) > 0 && (strings.Contains(resp.Message, "冲突") || strings.Contains(resp.Message, "已有进行中")),
			"错误消息应提示冲突或已有进行中申请，实际为: "+resp.Message)

		tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/reject", swapID),
			tc.Tokens["emp2"],
			gin.H{"remark": "不换了"},
			http.StatusOK,
		)

		_, resp = tc.CreateSwap(t, "emp1", "emp3", 0, 0, "申请3-被拒后重试", http.StatusOK)
		assert.Equal(t, 0, resp.Code, "冲突被拒后应能重新申请")
	})
}

func TestDisapproveRollback(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("经理驳回回滚", func(t *testing.T) {
		swapID, _ := tc.CreateSwap(t, "emp1", "emp2", 1, 1, "换班", http.StatusOK)

		tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/accept", swapID),
			tc.Tokens["emp2"],
			gin.H{"remark": "OK"},
			http.StatusOK,
		)
		tc.AssertSwapStatus(t, swapID, models.SwapStatusAccepted)

		tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/disapprove", swapID),
			tc.Tokens["manager"],
			gin.H{"remark": "排班不合理"},
			http.StatusOK,
		)
		tc.AssertSwapStatus(t, swapID, models.SwapStatusDisapproved)
		tc.AssertOperationLogs(t, models.OpTypeDisapproveSwap, 1)
		tc.AssertNotificationCount(t, "emp1", 3, models.NotifTypeSwapDisapproved)
		tc.AssertNotificationCount(t, "emp2", 3, models.NotifTypeSwapDisapproved)

		reqShift := tc.ShiftIDs["emp1"][1]
		tgtShift := tc.ShiftIDs["emp2"][1]
		tc.AssertShiftOwner(t, reqShift, "emp1")
		tc.AssertShiftOwner(t, tgtShift, "emp2")

		_, resp := tc.CreateSwap(t, "emp1", "emp2", 1, 1, "驳回后重新申请", http.StatusOK)
		assert.Equal(t, 0, resp.Code, "驳回后班次重新开放申请")
	})
}

func TestApprovalAuth(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("Admin审批鉴权", func(t *testing.T) {
		swapID, _ := tc.CreateSwap(t, "emp1", "emp2", 2, 2, "换班A", http.StatusOK)

		tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/accept", swapID),
			tc.Tokens["emp2"],
			gin.H{"remark": "OK"},
			http.StatusOK,
		)

		tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/approve", swapID),
			tc.Tokens["emp1"],
			gin.H{"remark": "员工无审批权限"},
			http.StatusForbidden,
		)

		_ = swapID
	})

	t.Run("经理审批通过", func(t *testing.T) {
		swapID2, _ := tc.CreateSwap(t, "emp1", "emp2", 3, 3, "换班B", http.StatusOK)
		tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/accept", swapID2),
			tc.Tokens["emp2"],
			gin.H{"remark": "OK"},
			http.StatusOK,
		)
		resp := tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/approve", swapID2),
			tc.Tokens["manager"],
			gin.H{"remark": "经理审批"},
			http.StatusOK,
		)
		assert.Equal(t, 0, resp.Code)
	})

	t.Run("管理员审批通过", func(t *testing.T) {
		swapID3, _ := tc.CreateSwap(t, "emp1", "emp3", 4, 4, "换班C", http.StatusOK)
		tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/accept", swapID3),
			tc.Tokens["emp3"],
			gin.H{"remark": "OK"},
			http.StatusOK,
		)
		resp := tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/approve", swapID3),
			tc.Tokens["admin"],
			gin.H{"remark": "管理员审批"},
			http.StatusOK,
		)
		assert.Equal(t, 0, resp.Code)
	})
}

func TestCancelFlow(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("取消换班流程", func(t *testing.T) {
		swapID, _ := tc.CreateSwap(t, "emp1", "emp2", 0, 0, "测试取消", http.StatusOK)

		tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/cancel", swapID),
			tc.Tokens["emp2"],
			nil,
			http.StatusForbidden,
		)

		resp := tc.MakeRequest(t, "POST",
			fmt.Sprintf("/api/swaps/%d/cancel", swapID),
			tc.Tokens["emp1"],
			nil,
			http.StatusOK,
		)
		assert.Equal(t, 0, resp.Code)
		tc.AssertSwapStatus(t, swapID, models.SwapStatusCanceled)
		tc.AssertNotificationCount(t, "emp2", 3, models.NotifTypeSwapCanceled)
	})
}

func TestNotificationAPI(t *testing.T) {
	tc := SetupTestEnv(t)

	tc.CreateSwap(t, "emp1", "emp2", 0, 0, "测试通知", http.StatusOK)

	t.Run("查询通知列表", func(t *testing.T) {
		resp := tc.MakeRequest(t, "GET", "/api/notifications", tc.Tokens["emp2"], nil, http.StatusOK)
		assert.Equal(t, 0, resp.Code)

		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok, "data should be map")
		items, ok := dataMap["items"].([]interface{})
		assert.True(t, ok)
		assert.GreaterOrEqual(t, len(items), 1, "应有至少1条通知")
	})

	t.Run("查询未读通知", func(t *testing.T) {
		resp := tc.MakeRequest(t, "GET", "/api/notifications?unread_only=true", tc.Tokens["emp1"], nil, http.StatusOK)
		assert.Equal(t, 0, resp.Code)
		dataMap, _ := resp.Data.(map[string]interface{})
		unreadCount, _ := dataMap["unread_count"].(float64)
		assert.GreaterOrEqual(t, unreadCount, float64(1))
	})

	t.Run("标记通知已读", func(t *testing.T) {
		resp := tc.MakeRequest(t, "POST", "/api/notifications/read-all", tc.Tokens["emp1"], nil, http.StatusOK)
		assert.Equal(t, 0, resp.Code)

		resp = tc.MakeRequest(t, "GET", "/api/notifications?unread_only=true", tc.Tokens["emp1"], nil, http.StatusOK)
		dataMap, _ := resp.Data.(map[string]interface{})
		unreadCount, _ := dataMap["unread_count"].(float64)
		assert.Equal(t, float64(0), unreadCount)
	})
}

func TestShiftManagement(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("员工查询班次列表", func(t *testing.T) {
		resp := tc.MakeRequest(t, "GET", "/api/shifts/my", tc.Tokens["emp1"], nil, http.StatusOK)
		assert.Equal(t, 0, resp.Code)
		shifts, _ := resp.Data.([]interface{})
		assert.GreaterOrEqual(t, len(shifts), 1)
	})

	t.Run("员工不能创建班次", func(t *testing.T) {
		body := gin.H{
			"user_id": tc.UserIDs["emp1"],
			"shift_date": "2099-01-01",
			"start_time": "09:00",
			"end_time":   "17:00",
		}
		tc.MakeRequest(t, "POST", "/api/shifts", tc.Tokens["emp1"], body, http.StatusForbidden)
	})

	t.Run("经理创建班次", func(t *testing.T) {
		body := gin.H{
			"user_id": tc.UserIDs["emp3"],
			"shift_date": "2099-01-01",
			"start_time": "09:00",
			"shift_type": "早班",
			"end_time":   "17:00",
			"location":   "办公室",
		}
		resp := tc.MakeRequest(t, "POST", "/api/shifts", tc.Tokens["manager"], body, http.StatusOK)
		assert.Equal(t, 0, resp.Code)
		tc.AssertOperationLogs(t, models.OpTypeCreateShift, 1)
	})
}

func TestOperationLogs(t *testing.T) {
	tc := SetupTestEnv(t)

	swapID, _ := tc.CreateSwap(t, "emp1", "emp2", 0, 0, "完整流程日志", http.StatusOK)
	tc.MakeRequest(t, "POST",
		fmt.Sprintf("/api/swaps/%d/accept", swapID),
		tc.Tokens["emp2"],
		gin.H{"remark": "同意"},
		http.StatusOK,
	)
	tc.MakeRequest(t, "POST",
		fmt.Sprintf("/api/swaps/%d/approve", swapID),
		tc.Tokens["manager"],
		gin.H{"remark": "通过"},
		http.StatusOK,
	)

	t.Run("员工只能看自己的日志", func(t *testing.T) {
		resp := tc.MakeRequest(t, "GET", "/api/logs/my", tc.Tokens["emp1"], nil, http.StatusOK)
		assert.Equal(t, 0, resp.Code)
		logs, _ := resp.Data.([]interface{})
		assert.GreaterOrEqual(t, len(logs), 1)
	})

	t.Run("经理看全部日志", func(t *testing.T) {
		resp := tc.MakeRequest(t, "GET", "/api/logs", tc.Tokens["manager"], nil, http.StatusOK)
		assert.Equal(t, 0, resp.Code)
		logs, _ := resp.Data.([]interface{})
		assert.GreaterOrEqual(t, len(logs), 3)
	})

	t.Run("员工看全部日志被拒绝", func(t *testing.T) {
		tc.MakeRequest(t, "GET", "/api/logs", tc.Tokens["emp1"], nil, http.StatusForbidden)
	})
}

func TestExistingProtocolCompatibility(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("保持现有API路径不变", func(t *testing.T) {
		paths := []struct {
			method string
			path   string
			token  string
			code   int
		}{
			{"POST", "/api/auth/register", "", http.StatusBadRequest},
			{"POST", "/api/auth/login", "", http.StatusBadRequest},
			{"GET", "/api/auth/me", "emp1", http.StatusOK},
			{"GET", "/api/shifts", "emp1", http.StatusOK},
			{"GET", "/api/shifts/my", "emp1", http.StatusOK},
			{"GET", "/api/swaps", "emp1", http.StatusOK},
			{"GET", "/api/logs/my", "emp1", http.StatusOK},
			{"GET", "/api/notifications", "emp1", http.StatusOK},
		}
		for _, p := range paths {
			token := ""
			if p.token != "" {
				token = tc.Tokens[p.token]
			}
			w := tc.MakeRequest(t, p.method, p.path, token, nil, p.code)
			_ = w
		}
	})

	t.Run("换班状态值保持一致", func(t *testing.T) {
		swapID, _ := tc.CreateSwap(t, "emp1", "emp2", 4, 4, "状态测试", http.StatusOK)
		resp := tc.MakeRequest(t, "GET", fmt.Sprintf("/api/swaps/%d", swapID), tc.Tokens["emp1"], nil, http.StatusOK)
		var swap models.SwapRequest
		dataBytes, _ := json.Marshal(resp.Data)
		json.Unmarshal(dataBytes, &swap)
		assert.Equal(t, models.SwapStatusPending, swap.Status)
		assert.NotNil(t, swap.Requester)
		assert.NotNil(t, swap.TargetUser)
	})
}
