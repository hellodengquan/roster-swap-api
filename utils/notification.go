package utils

import (
	"encoding/json"
	"fmt"
	"roster-swap-api/config"
	"roster-swap-api/models"
	"time"
)

type SwapNotifMetadata struct {
	RequesterName   string `json:"requester_name"`
	TargetUserName  string `json:"target_user_name"`
	ApproverName    string `json:"approver_name,omitempty"`
	RequesterShift  string `json:"requester_shift"`
	TargetShift     string `json:"target_shift"`
	Reason          string `json:"reason,omitempty"`
	Remark          string `json:"remark,omitempty"`
}

func CreateInAppNotification(userID uint, notifType models.NotificationType, title, content string, swapID *uint, metadata interface{}) error {
	metaJSON := ""
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			metaJSON = string(b)
		}
	}

	notif := &models.Notification{
		UserID:        userID,
		Type:          notifType,
		Channel:       models.ChannelInApp,
		Title:         title,
		Content:       content,
		SwapRequestID: swapID,
		Status:        models.NotifStatusPending,
		Metadata:      metaJSON,
	}

	if err := config.DB.Create(notif).Error; err != nil {
		return err
	}

	now := time.Now()
	config.DB.Model(notif).Updates(map[string]interface{}{
		"status":  models.NotifStatusSent,
		"sent_at": &now,
	})

	return nil
}

func DispatchEmailNotification(userID uint, notifType models.NotificationType, title, content string, swapID *uint) error {
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在: %v", err)
	}

	if user.Email == "" {
		return fmt.Errorf("用户 %s 未设置邮箱", user.Username)
	}

	fmt.Printf("[邮件通知] To: %s (%s)\n", user.Email, user.Name)
	fmt.Printf("  主题: %s\n", title)
	fmt.Printf("  内容: %s\n", content)
	fmt.Println("  --- (模拟邮件发送成功)")

	notif := &models.Notification{
		UserID:        userID,
		Type:          notifType,
		Channel:       models.ChannelEmail,
		Title:         title,
		Content:       content,
		SwapRequestID: swapID,
		Status:        models.NotifStatusSent,
	}
	now := time.Now()
	notif.SentAt = &now
	return config.DB.Create(notif).Error
}

func DispatchIMNotification(userID uint, notifType models.NotificationType, title, content string, swapID *uint) error {
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		return fmt.Errorf("用户不存在: %v", err)
	}

	fmt.Printf("[即时消息通知] To: %s (手机号: %s)\n", user.Name, user.Phone)
	fmt.Printf("  标题: %s\n", title)
	fmt.Printf("  内容: %s\n", content)
	fmt.Println("  --- (模拟IM消息发送成功)")

	notif := &models.Notification{
		UserID:        userID,
		Type:          notifType,
		Channel:       models.ChannelIM,
		Title:         title,
		Content:       content,
		SwapRequestID: swapID,
		Status:        models.NotifStatusSent,
	}
	now := time.Now()
	notif.SentAt = &now
	return config.DB.Create(notif).Error
}

func NotifyUser(userID uint, notifType models.NotificationType, title, content string, swapID *uint, metadata interface{}) {
	CreateInAppNotification(userID, notifType, title, content, swapID, metadata)

	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		return
	}

	if user.Email != "" {
		DispatchEmailNotification(userID, notifType, title, content, swapID)
	}

	if user.Phone != "" {
		DispatchIMNotification(userID, notifType, title, content, swapID)
	}
}

func BuildShiftDescription(shift *models.Shift) string {
	return fmt.Sprintf("%s %s-%s",
		shift.ShiftDate.Format("2006-01-02"),
		shift.StartTime,
		shift.EndTime)
}
