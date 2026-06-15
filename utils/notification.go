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

func GetUserPreference(userID uint) *models.UserPreference {
	var pref models.UserPreference
	err := config.DB.Where("user_id = ?", userID).First(&pref).Error
	if err != nil {
		pref = models.UserPreference{
			UserID:                   userID,
			InAppNotifyEnabled:       true,
			EmailNotifyEnabled:       true,
			IMNotifyEnabled:          true,
			SwapRequestNotifyEnabled: true,
			ApprovalNotifyEnabled:    true,
		}
		pref.SetNotifyChannels([]models.NotificationChannel{
			models.ChannelInApp, models.ChannelEmail, models.ChannelIM,
		})
		config.DB.Create(&pref)
	}
	return &pref
}

func SaveUserPreference(pref *models.UserPreference) error {
	return config.DB.Save(pref).Error
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

	if user.Phone == "" {
		return fmt.Errorf("用户 %s 未设置手机号", user.Username)
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

func GetEnabledChannels(pref *models.UserPreference,
	notifType models.NotificationType,
	forcedChannels ...models.NotificationChannel) []models.NotificationChannel {

	if len(forcedChannels) > 0 {
		return forcedChannels
	}

	prefChannels := pref.GetNotifyChannels()

	isSwapRelated := notifType == models.NotifTypeSwapCreated ||
		notifType == models.NotifTypeSwapAccepted ||
		notifType == models.NotifTypeSwapRejected ||
		notifType == models.NotifTypeSwapCanceled

	isApprovalRelated := notifType == models.NotifTypeSwapApproved ||
		notifType == models.NotifTypeSwapDisapproved ||
		notifType == models.NotifTypeSwapCompleted

	enabled := false
	switch {
	case isSwapRelated:
		enabled = pref.SwapRequestNotifyEnabled
	case isApprovalRelated:
		enabled = pref.ApprovalNotifyEnabled
	default:
		enabled = true
	}

	if !enabled {
		return []models.NotificationChannel{}
	}

	result := make([]models.NotificationChannel, 0, len(prefChannels))
	for _, ch := range prefChannels {
		switch ch {
		case models.ChannelInApp:
			if pref.InAppNotifyEnabled {
				result = append(result, ch)
			}
		case models.ChannelEmail:
			if pref.EmailNotifyEnabled {
				result = append(result, ch)
			}
		case models.ChannelIM:
			if pref.IMNotifyEnabled {
				result = append(result, ch)
			}
		}
	}

	if len(result) == 0 {
		result = []models.NotificationChannel{models.ChannelInApp}
	}

	return result
}

func NotifyUser(userID uint, notifType models.NotificationType, title, content string, swapID *uint, metadata interface{}, channels ...models.NotificationChannel) {

	pref := GetUserPreference(userID)
	enabledChannels := GetEnabledChannels(pref, notifType, channels...)

	for _, ch := range enabledChannels {
		switch ch {
		case models.ChannelInApp:
			CreateInAppNotification(userID, notifType, title, content, swapID, metadata)
		case models.ChannelEmail:
			DispatchEmailNotification(userID, notifType, title, content, swapID)
		case models.ChannelIM:
			DispatchIMNotification(userID, notifType, title, content, swapID)
		}
	}
}

func BuildShiftDescription(shift *models.Shift) string {
	return fmt.Sprintf("%s %s-%s",
		shift.ShiftDate.Format("2006-01-02"),
		shift.StartTime,
		shift.EndTime)
}
