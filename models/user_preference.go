package models

import (
	"encoding/json"
	"time"
)

type NotificationPreference struct {
	EnabledChannels []NotificationChannel `json:"enabled_channels"`
	QuietHoursStart string                `json:"quiet_hours_start,omitempty"`
	QuietHoursEnd   string                `json:"quiet_hours_end,omitempty"`
}

type UserPreference struct {
	ID                        uint      `gorm:"primaryKey" json:"id"`
	UserID                    uint      `gorm:"unique;not null;index" json:"user_id"`
	User                      User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	NotifyChannelsJSON        string    `gorm:"column:notify_channels_json;type:text;default:'[\"inapp\",\"email\",\"im\"]'" json:"-"`
	NotificationQuietHours    string    `gorm:"size:50" json:"notification_quiet_hours,omitempty"`
	SwapRequestNotifyEnabled  bool      `gorm:"default:true" json:"swap_request_notify_enabled"`
	ApprovalNotifyEnabled     bool      `gorm:"default:true" json:"approval_notify_enabled"`
	EmailNotifyEnabled        bool      `gorm:"default:true" json:"email_notify_enabled"`
	IMNotifyEnabled           bool      `gorm:"default:true" json:"im_notify_enabled"`
	InAppNotifyEnabled        bool      `gorm:"default:true" json:"inapp_notify_enabled"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

func (up *UserPreference) GetNotifyChannels() []NotificationChannel {
	if up.NotifyChannelsJSON == "" {
		result := []NotificationChannel{ChannelInApp}
		if up.EmailNotifyEnabled {
			result = append(result, ChannelEmail)
		}
		if up.IMNotifyEnabled {
			result = append(result, ChannelIM)
		}
		return result
	}
	var channels []NotificationChannel
	if err := json.Unmarshal([]byte(up.NotifyChannelsJSON), &channels); err != nil {
		return []NotificationChannel{ChannelInApp}
	}
	return channels
}

func (up *UserPreference) SetNotifyChannels(channels []NotificationChannel) {
	if b, err := json.Marshal(channels); err == nil {
		up.NotifyChannelsJSON = string(b)
	}
	for _, ch := range channels {
		switch ch {
		case ChannelEmail:
			up.EmailNotifyEnabled = true
		case ChannelIM:
			up.IMNotifyEnabled = true
		case ChannelInApp:
			up.InAppNotifyEnabled = true
		}
	}
}
