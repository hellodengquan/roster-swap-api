package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"roster-swap-api/config"
	"roster-swap-api/middleware"
	"roster-swap-api/models"
	"roster-swap-api/routes"
	"roster-swap-api/utils"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type TestContext struct {
	Router    *gin.Engine
	DB        *gorm.DB
	Tokens    map[string]string
	UserIDs   map[string]uint
	ShiftIDs  map[string][]uint
	SwapIDs   []uint
}

type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func SetupTestEnv(t *testing.T) *TestContext {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.UserPreference{},
		&models.Shift{},
		&models.SwapRequest{},
		&models.OperationLog{},
		&models.Notification{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	config.DB = db
	config.AppConfig = config.Config{
		JWTSecret:     "test-secret-key",
		JWTExpireHours: 24,
		Port:          "8080",
		DBPath:        ":memory:",
	}

	passwordHash, _ := utils.HashPassword("123456")

	users := map[string]*models.User{
		"admin": {
			Username: "admin", Password: passwordHash,
			Name: "管理员", Role: models.RoleAdmin,
			Email: "admin@test.com", Phone: "13800000000",
		},
		"manager": {
			Username: "manager", Password: passwordHash,
			Name: "张经理", Role: models.RoleManager,
			Email: "manager@test.com", Phone: "13800000001",
		},
		"emp1": {
			Username: "emp1", Password: passwordHash,
			Name: "李小明", Role: models.RoleEmployee, Department: "运营部",
			Email: "emp1@test.com", Phone: "13800000011",
		},
		"emp2": {
			Username: "emp2", Password: passwordHash,
			Name: "王小红", Role: models.RoleEmployee, Department: "运营部",
			Email: "emp2@test.com", Phone: "13800000012",
		},
		"emp3": {
			Username: "emp3", Password: passwordHash,
			Name: "赵小刚", Role: models.RoleEmployee, Department: "运营部",
			Email: "emp3@test.com", Phone: "13800000013",
		},
	}

	userIDs := make(map[string]uint)
	for key, u := range users {
		db.Create(u)
		userIDs[key] = u.ID
	}

	shiftIDs := make(map[string][]uint)
	today := time.Now()

	for i := 0; i < 5; i++ {
		date := today.AddDate(0, 0, i)
		hour := 8 + (i * 4) % 16

		s1 := &models.Shift{
			UserID:    userIDs["emp1"],
			ShiftDate: date,
			StartTime: fmt.Sprintf("%02d:00", hour),
			EndTime:   fmt.Sprintf("%02d:00", hour+8),
			ShiftType: "早班", Status: models.ShiftStatusActive,
			Location: "总部",
		}
		db.Create(s1)
		shiftIDs["emp1"] = append(shiftIDs["emp1"], s1.ID)

		s2 := &models.Shift{
			UserID:    userIDs["emp2"],
			ShiftDate: date,
			StartTime: fmt.Sprintf("%02d:00", (hour+4)%24),
			EndTime:   fmt.Sprintf("%02d:00", (hour+12)%24),
			ShiftType: "中班", Status: models.ShiftStatusActive,
			Location: "总部",
		}
		db.Create(s2)
		shiftIDs["emp2"] = append(shiftIDs["emp2"], s2.ID)

		s3 := &models.Shift{
			UserID:    userIDs["emp3"],
			ShiftDate: date,
			StartTime: fmt.Sprintf("%02d:00", (hour+8)%24),
			EndTime:   fmt.Sprintf("%02d:00", (hour+16)%24),
			ShiftType: "晚班", Status: models.ShiftStatusActive,
			Location: "总部",
		}
		db.Create(s3)
		shiftIDs["emp3"] = append(shiftIDs["emp3"], s3.ID)
	}

	r := gin.New()
	routes.SetupRoutes(r)

	tokens := make(map[string]string)
	for key, u := range users {
		token, _ := utils.GenerateToken(u)
		tokens[key] = token
	}

	return &TestContext{
		Router:   r,
		DB:       db,
		Tokens:   tokens,
		UserIDs:  userIDs,
		ShiftIDs: shiftIDs,
	}
}

func (tc *TestContext) MakeRequest(t *testing.T, method, path string, token string, body interface{}, expectedCode int) *APIResponse {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, path, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	tc.Router.ServeHTTP(w, req)

	assert.Equal(t, expectedCode, w.Code, "Expected status %d but got %d. Body: %s",
		expectedCode, w.Code, w.Body.String())

	var resp APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil && w.Code != http.StatusNoContent {
		t.Fatalf("Failed to parse response: %v, body: %s", err, w.Body.String())
	}
	return &resp
}

func (tc *TestContext) ParseData(resp *APIResponse, target interface{}) {
	dataBytes, _ := json.Marshal(resp.Data)
	json.Unmarshal(dataBytes, target)
}

func (tc *TestContext) CreateSwap(t *testing.T, requesterKey string, targetKey string,
	reqShiftIdx int, targetShiftIdx int, reason string, expectedStatus int) (uint, *APIResponse) {
	body := gin.H{
		"target_user_id":      tc.UserIDs[targetKey],
		"requester_shift_id":  tc.ShiftIDs[requesterKey][reqShiftIdx],
		"target_shift_id":     tc.ShiftIDs[targetKey][targetShiftIdx],
		"reason":              reason,
	}
	resp := tc.MakeRequest(t, "POST", "/api/swaps", tc.Tokens[requesterKey], body, expectedStatus)
	if resp.Code == 0 && resp.Data != nil {
		var swap models.SwapRequest
		dataBytes, _ := json.Marshal(resp.Data)
		json.Unmarshal(dataBytes, &swap)
		tc.SwapIDs = append(tc.SwapIDs, swap.ID)
		return swap.ID, resp
	}
	return 0, resp
}

func (tc *TestContext) AssertNotificationCount(t *testing.T, userKey string, expectedCount int64, notifType ...models.NotificationType) {
	var count int64
	q := tc.DB.Model(&models.Notification{}).Where("user_id = ?", tc.UserIDs[userKey])
	if len(notifType) > 0 {
		q = q.Where("type = ?", notifType[0])
	}
	q.Count(&count)
	assert.Equal(t, expectedCount, count, "用户 %s 通知数量不匹配", userKey)
}

func (tc *TestContext) AssertSwapStatus(t *testing.T, swapID uint, expectedStatus models.SwapStatus) {
	var swap models.SwapRequest
	tc.DB.First(&swap, swapID)
	assert.Equal(t, expectedStatus, swap.Status, "换班状态不匹配")
}

func (tc *TestContext) AssertOperationLogs(t *testing.T, opType models.OperationType, expectedCount int64) {
	var count int64
	tc.DB.Model(&models.OperationLog{}).
		Where("operation_type = ?", opType).
		Count(&count)
	assert.Equal(t, expectedCount, count, "操作日志 %s 数量不匹配", opType)
}

func (tc *TestContext) AssertShiftOwner(t *testing.T, shiftID uint, expectedUserKey string) {
	var shift models.Shift
	tc.DB.First(&shift, shiftID)
	assert.Equal(t, tc.UserIDs[expectedUserKey], shift.UserID, "班次所属人不匹配")
}

var _ = middleware.AuthMiddleware
