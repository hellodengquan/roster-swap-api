package models

import (
	"time"
)

type SwapStatus string

const (
	SwapStatusPending     SwapStatus = "pending"
	SwapStatusAccepted    SwapStatus = "accepted"
	SwapStatusRejected    SwapStatus = "rejected"
	SwapStatusApproved    SwapStatus = "approved"
	SwapStatusDisapproved SwapStatus = "disapproved"
	SwapStatusCanceled    SwapStatus = "canceled"
	SwapStatusCompleted   SwapStatus = "completed"
)

type SwapRequest struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	RequesterID      uint       `gorm:"not null;index" json:"requester_id"`
	Requester        User       `gorm:"foreignKey:RequesterID" json:"requester,omitempty"`
	TargetUserID     uint       `gorm:"not null;index" json:"target_user_id"`
	TargetUser       User       `gorm:"foreignKey:TargetUserID" json:"target_user,omitempty"`
	RequesterShiftID uint       `gorm:"not null;index" json:"requester_shift_id"`
	RequesterShift   Shift      `gorm:"foreignKey:RequesterShiftID" json:"requester_shift,omitempty"`
	TargetShiftID    uint       `gorm:"not null;index" json:"target_shift_id"`
	TargetShift      Shift      `gorm:"foreignKey:TargetShiftID" json:"target_shift,omitempty"`
	Reason           string     `gorm:"size:500;not null" json:"reason"`
	Status           SwapStatus `gorm:"size:20;default:'pending';index" json:"status"`
	TargetUserRemark string     `gorm:"size:500" json:"target_user_remark"`
	ApproverID       *uint      `gorm:"index" json:"approver_id"`
	Approver         *User      `gorm:"foreignKey:ApproverID" json:"approver,omitempty"`
	ApprovalRemark   string     `gorm:"size:500" json:"approval_remark"`
	TargetUserAt     *time.Time `json:"target_user_at"`
	ApprovedAt       *time.Time `json:"approved_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
