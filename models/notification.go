package models

import (
	"time"
)

type NotificationType string

const (
	NotifTypeSwapCreated     NotificationType = "swap_created"
	NotifTypeSwapAccepted    NotificationType = "swap_accepted"
	NotifTypeSwapRejected    NotificationType = "swap_rejected"
	NotifTypeSwapApproved    NotificationType = "swap_approved"
	NotifTypeSwapDisapproved NotificationType = "swap_disapproved"
	NotifTypeSwapCanceled    NotificationType = "swap_canceled"
	NotifTypeSwapCompleted   NotificationType = "swap_completed"
)

type NotificationChannel string

const (
	ChannelInApp NotificationChannel = "inapp"
	ChannelEmail NotificationChannel = "email"
	ChannelIM    NotificationChannel = "im"
)

type NotificationStatus string

const (
	NotifStatusPending NotificationStatus = "pending"
	NotifStatusSent    NotificationStatus = "sent"
	NotifStatusRead    NotificationStatus = "read"
	NotifStatusFailed  NotificationStatus = "failed"
)

type Notification struct {
	ID            uint                `gorm:"primaryKey" json:"id"`
	UserID        uint                `gorm:"not null;index" json:"user_id"`
	User          User                `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Type          NotificationType    `gorm:"size:50;not null;index" json:"type"`
	Channel       NotificationChannel `gorm:"size:20;not null;default:'inapp'" json:"channel"`
	Title         string              `gorm:"size:200;not null" json:"title"`
	Content       string              `gorm:"size:2000;not null" json:"content"`
	SwapRequestID *uint               `gorm:"index" json:"swap_request_id"`
	SwapRequest   *SwapRequest        `gorm:"foreignKey:SwapRequestID" json:"swap_request,omitempty"`
	Status        NotificationStatus  `gorm:"size:20;default:'pending';index" json:"status"`
	ReadAt        *time.Time          `json:"read_at"`
	SentAt        *time.Time          `json:"sent_at"`
	FailedReason  string              `gorm:"size:500" json:"failed_reason"`
	Metadata      string              `gorm:"size:2000" json:"metadata"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}
