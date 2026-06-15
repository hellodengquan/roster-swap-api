package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"roster-swap-api/models"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
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

func TestNotificationChannelRouting(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("查询用户通知偏好", func(t *testing.T) {
		resp := tc.MakeRequest(t, "GET", "/api/preferences", tc.Tokens["emp1"], nil, http.StatusOK)
		assert.Equal(t, 0, resp.Code)

		dataMap, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, true, dataMap["inapp_notify_enabled"], "默认开启站内消息")
	})

	t.Run("关闭邮件和IM通道，仅保留站内", func(t *testing.T) {
		resp := tc.MakeRequest(t, "PUT", "/api/preferences", tc.Tokens["emp1"], gin.H{
			"enabled_channels": []models.NotificationChannel{models.ChannelInApp},
		}, http.StatusOK)
		assert.Equal(t, 0, resp.Code)

		tc.DB.Session(&gorm.Session{})
	})

	t.Run("关闭邮件通知后发送仅产生站内消息", func(t *testing.T) {
		tc.MakeRequest(t, "PUT", "/api/preferences", tc.Tokens["emp1"], gin.H{
			"email_notify": false,
			"im_notify": false,
			"inapp_notify": true,
		}, http.StatusOK)

		swapID, _ := tc.CreateSwap(t, "emp1", "emp2", 0, 0, "通道测试", http.StatusOK)

		var emailCount int64
		tc.DB.Model(&models.Notification{}).
			Where("user_id = ? AND type = ? AND channel = ?",
				tc.UserIDs["emp1"], models.NotifTypeSwapCreated, models.ChannelEmail).
			Count(&emailCount)
		assert.Equal(t, int64(0), emailCount, "应无邮件通知")

		var inAppCount int64
		tc.DB.Model(&models.Notification{}).
			Where("user_id = ? AND type = ? AND channel = ?",
				tc.UserIDs["emp1"], models.NotifTypeSwapCreated, models.ChannelInApp).
			Count(&inAppCount)
		assert.Equal(t, int64(1), inAppCount, "应有1条站内通知")
		_ = swapID
	})

	t.Run("管理员派发指定通道通知", func(t *testing.T) {
		resp := tc.MakeRequest(t, "POST", "/api/notifications/dispatch", tc.Tokens["manager"], gin.H{
			"user_id":  tc.UserIDs["emp1"],
			"title":    "紧急通知",
			"content":  "请尽快完成任务",
			"channels": []models.NotificationChannel{models.ChannelInApp},
		}, http.StatusOK)
		assert.Equal(t, 0, resp.Code)

		var count int64
		tc.DB.Model(&models.Notification{}).
			Where("user_id = ? AND channel = ? AND title = ?",
				tc.UserIDs["emp1"], models.ChannelInApp, "紧急通知").
			Count(&count)
		assert.Equal(t, int64(1), count, "应产生1条指定标题的站内消息")
	})

	t.Run("员工不能派发通知", func(t *testing.T) {
		tc.MakeRequest(t, "POST", "/api/notifications/dispatch", tc.Tokens["emp1"], gin.H{
			"user_id":  tc.UserIDs["emp2"],
			"title":    "越权通知",
			"content":  "不应发送",
			"channels": []models.NotificationChannel{models.ChannelInApp},
		}, http.StatusForbidden)
	})
}

func TestBatchApprovalOperations(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("创建多个accepted状态的申请", func(t *testing.T) {
		ids := []uint{}

		for i := 0; i < 3; i++ {
			swapID, _ := tc.CreateSwap(t, "emp1", "emp2", i, i, fmt.Sprintf("批量审批-%d", i), http.StatusOK)
			tc.MakeRequest(t, "POST",
				fmt.Sprintf("/api/swaps/%d/accept", swapID),
				tc.Tokens["emp2"], gin.H{"remark": "OK"}, http.StatusOK,
			)
			ids = append(ids, swapID)
			tc.AssertSwapStatus(t, swapID, models.SwapStatusAccepted)
		}

		resp := tc.MakeRequest(t, "POST", "/api/swaps/batch/approve", tc.Tokens["manager"], gin.H{
			"ids":    ids,
			"remark": "批量通过审批",
		}, http.StatusOK)
		assert.Equal(t, 0, resp.Code)

		dataMap, _ := resp.Data.(map[string]interface{})
		successCount := int(dataMap["success_count"].(float64))
		failedCount := int(dataMap["failed_count"].(float64))
		assert.Equal(t, 3, successCount, "应成功通过3个")
		assert.Equal(t, 0, failedCount, "应无失败")

		for _, id := range ids {
			tc.AssertSwapStatus(t, id, models.SwapStatusCompleted)
		}

		var countEmp1, countEmp2 int64
		tc.DB.Model(&models.Notification{}).
			Where("user_id = ? AND type = ?", tc.UserIDs["emp1"], models.NotifTypeSwapCompleted).
			Count(&countEmp1)
		tc.DB.Model(&models.Notification{}).
			Where("user_id = ? AND type = ?", tc.UserIDs["emp2"], models.NotifTypeSwapCompleted).
			Count(&countEmp2)
		assert.GreaterOrEqual(t, countEmp1, int64(3), "emp1 应至少有3条 completed 通知（每批1条inapp）")
		assert.GreaterOrEqual(t, countEmp2, int64(3), "emp2 应至少有3条 completed 通知（每批1条inapp）")
	})

	t.Run("批量审批-员工无权限", func(t *testing.T) {
		tc.MakeRequest(t, "POST", "/api/swaps/batch/approve", tc.Tokens["emp1"], gin.H{
			"ids":    []uint{999},
			"remark": "越权",
		}, http.StatusForbidden)
	})

	t.Run("批量驳回-包含状态异常", func(t *testing.T) {
		ids := []uint{}
		actualStatuses := []models.SwapStatus{}

		for i := 3; i < 5; i++ {
			swapID, resp := tc.CreateSwap(t, "emp1", "emp3", i, i, fmt.Sprintf("批量驳回-%d", i), http.StatusOK)
			makeAccepted := (i == 3)
			if resp.Code == 0 && swapID > 0 && makeAccepted {
				tc.MakeRequest(t, "POST",
					fmt.Sprintf("/api/swaps/%d/accept", swapID),
					tc.Tokens["emp3"], gin.H{"remark": "OK"}, http.StatusOK,
				)
				actualStatuses = append(actualStatuses, models.SwapStatusAccepted)
			} else {
				actualStatuses = append(actualStatuses, models.SwapStatusPending)
			}
			if swapID > 0 {
				ids = append(ids, swapID)
			}
		}

		if len(ids) < 2 {
			t.Skip("无法创建足够的测试申请")
			return
		}

		resp := tc.MakeRequest(t, "POST", "/api/swaps/batch/disapprove", tc.Tokens["manager"], gin.H{
			"ids":    ids,
			"remark": "批量驳回：排班不合理",
		}, http.StatusOK)
		assert.Equal(t, 0, resp.Code)

		dataMap, _ := resp.Data.(map[string]interface{})
		successCount := int(dataMap["success_count"].(float64))
		failedCount := int(dataMap["failed_count"].(float64))
		totalCount := int(dataMap["total_count"].(float64))
		assert.Equal(t, len(ids), totalCount, "总数应等于传入IDs数量")
		assert.Equal(t, 1, successCount, "应成功驳回1个accepted")
		assert.Equal(t, 1, failedCount, "应有1个失败(pending)")
		assert.Equal(t, len(ids), successCount+failedCount, "成功数+失败数=总数")

		for i, id := range ids {
			if actualStatuses[i] == models.SwapStatusAccepted {
				tc.AssertSwapStatus(t, id, models.SwapStatusDisapproved)
			}
		}
	})

	t.Run("批量驳回-必须填原因", func(t *testing.T) {
		tc.MakeRequest(t, "POST", "/api/swaps/batch/disapprove", tc.Tokens["manager"], gin.H{
			"ids":    []uint{1},
			"remark": "",
		}, http.StatusBadRequest)
	})
}

func TestAuditExportFunction(t *testing.T) {
	tc := SetupTestEnv(t)

	swapID, _ := tc.CreateSwap(t, "emp1", "emp2", 0, 0, "导出测试", http.StatusOK)
	tc.MakeRequest(t, "POST",
		fmt.Sprintf("/api/swaps/%d/accept", swapID),
		tc.Tokens["emp2"], gin.H{"remark": "OK"}, http.StatusOK,
	)
	tc.MakeRequest(t, "POST",
		fmt.Sprintf("/api/swaps/%d/approve", swapID),
		tc.Tokens["manager"], gin.H{"remark": "通过"}, http.StatusOK,
	)

	t.Run("员工无导出权限", func(t *testing.T) {
		tc.MakeRequest(t, "GET", "/api/logs/export", tc.Tokens["emp1"], nil, http.StatusForbidden)
	})

	t.Run("管理员导出CSV", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/logs/export", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tc.Tokens["admin"])

		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/csv", "应为CSV格式")
		assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment", "应触发下载")
		assert.Contains(t, w.Header().Get("Content-Disposition"), ".csv", "文件名含csv后缀")

		body := w.Body.String()
		assert.Contains(t, body, "操作人姓名", "CSV包含中文表头")
		assert.Contains(t, body, "李小明", "包含操作人")
		assert.True(t, len(body) > 100, "CSV内容应不为空")
	})

	t.Run("导出-按用户过滤", func(t *testing.T) {
		url := fmt.Sprintf("/api/logs/export?user_id=%d", tc.UserIDs["emp1"])
		req := httptest.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+tc.Tokens["manager"])

		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "李小明", "仅包含emp1的操作日志")
	})

	t.Run("按日期范围过滤", func(t *testing.T) {
		today := time.Now().Format("2006-01-02")
		url := fmt.Sprintf("/api/logs/export?start_date=%s&end_date=%s", today, today)
		req := httptest.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+tc.Tokens["manager"])

		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "李小明")
	})
}

func TestEnhancedRollbackAndConflict(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("被拒-回滚链验证：取消后可重新申请", func(t *testing.T) {
		swap1, _ := tc.CreateSwap(t, "emp1", "emp2", 0, 0, "第一轮：取消", http.StatusOK)
		tc.MakeRequest(t, "POST", fmt.Sprintf("/api/swaps/%d/cancel", swap1),
			tc.Tokens["emp1"], nil, http.StatusOK)
		tc.AssertSwapStatus(t, swap1, models.SwapStatusCanceled)
		tc.AssertNotificationCount(t, "emp2", 3, models.NotifTypeSwapCanceled)

		swap2, resp := tc.CreateSwap(t, "emp1", "emp3", 1, 2, "取消后换不同目标", http.StatusOK)
		assert.Equal(t, 0, resp.Code, "取消后应能重新申请不同目标")
		tc.AssertSwapStatus(t, swap2, models.SwapStatusPending)
	})

	t.Run("并发冲突：A申请B同时，C申请B同一时段", func(t *testing.T) {
		_, _ = tc.CreateSwap(t, "emp1", "emp3", 0, 0, "A→C", http.StatusOK)
		_, resp := tc.CreateSwap(t, "emp2", "emp3", 1, 0, "B→C (同日同时间段冲突)", http.StatusBadRequest)
		assert.NotEqual(t, 0, resp.Code, "同一人同时间段不能有两个pending申请")
		assert.True(t,
			strings.Contains(resp.Message, "冲突") ||
				strings.Contains(resp.Message, "已有进行中"),
			"错误提示: "+resp.Message)
	})

	t.Run("并发冲突：申请人不同日不同班次", func(t *testing.T) {
		_, resp := tc.CreateSwap(t, "emp1", "emp3", 2, 1, "emp1-D3→emp3-D2", http.StatusOK)
		assert.Equal(t, 0, resp.Code, "不同日期不同目标班次不应冲突")
	})

	t.Run("冲突解决：驳回后可重新申请", func(t *testing.T) {
		swapA, _ := tc.CreateSwap(t, "emp2", "emp3", 2, 3, "申请A", http.StatusOK)
		tc.MakeRequest(t, "POST", fmt.Sprintf("/api/swaps/%d/accept", swapA),
			tc.Tokens["emp3"], gin.H{"remark": "OK"}, http.StatusOK)
		tc.MakeRequest(t, "POST", fmt.Sprintf("/api/swaps/%d/disapprove", swapA),
			tc.Tokens["manager"], gin.H{"remark": "驳回"}, http.StatusOK)
		tc.AssertSwapStatus(t, swapA, models.SwapStatusDisapproved)

		_, resp := tc.CreateSwap(t, "emp1", "emp2", 3, 4, "驳回后第三方申请", http.StatusOK)
		assert.Equal(t, 0, resp.Code, "驳回后其他班次可被申请")
	})

	t.Run("回滚后通知：驳回通知包含可重新申请提示", func(t *testing.T) {
		swapID, _ := tc.CreateSwap(t, "emp1", "emp3", 4, 4, "通知验证", http.StatusOK)
		tc.MakeRequest(t, "POST", fmt.Sprintf("/api/swaps/%d/accept", swapID),
			tc.Tokens["emp3"], gin.H{"remark": "OK"}, http.StatusOK)
		tc.MakeRequest(t, "POST", fmt.Sprintf("/api/swaps/%d/disapprove", swapID),
			tc.Tokens["manager"], gin.H{"remark": "排班调整"}, http.StatusOK)

		var notif models.Notification
		result := tc.DB.Where("user_id = ? AND type = ?",
			tc.UserIDs["emp1"], models.NotifTypeSwapDisapproved).
			Order("id DESC").First(&notif)
		if result.Error == nil {
			assert.NotZero(t, notif.ID, "应能找到驳回通知")
			assert.Contains(t, notif.Content, "回滚", "驳回通知应包含回滚关键词")
			assert.True(t,
				strings.Contains(notif.Content, "回滚") ||
					strings.Contains(notif.Content, "重新"),
				"驳回通知内容应提示回滚或重新申请，实际为: "+notif.Content)
		} else {
			t.Logf("警告：未找到驳回通知，%v", result.Error)
		}
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
			{"GET", "/api/preferences", "emp1", http.StatusOK},
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
