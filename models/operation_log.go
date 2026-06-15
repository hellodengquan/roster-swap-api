package models

import (
	"time"
)

type OperationType string

const (
	OpTypeCreateSwap   OperationType = "create_swap"
	OpTypeAcceptSwap   OperationType = "accept_swap"
	OpTypeRejectSwap   OperationType = "reject_swap"
	OpTypeApproveSwap  OperationType = "approve_swap"
	OpTypeDisapproveSwap OperationType = "disapprove_swap"
	OpTypeCancelSwap   OperationType = "cancel_swap"
	OpTypeCompleteSwap OperationType = "complete_swap"
	OpTypeCreateShift  OperationType = "create_shift"
	OpTypeUpdateShift  OperationType = "update_shift"
	OpTypeDeleteShift  OperationType = "delete_shift"
)

type OperationLog struct {
	ID              uint          `gorm:"primaryKey" json:"id"`
	UserID          uint          `gorm:"not null;index" json:"user_id"`
	User            User          `gorm:"foreignKey:UserID" json:"user,omitempty"`
	OperationType   OperationType `gorm:"size:50;not null;index" json:"operation_type"`
	SwapRequestID   *uint         `gorm:"index" json:"swap_request_id"`
	SwapRequest     *SwapRequest  `gorm:"foreignKey:SwapRequestID" json:"swap_request,omitempty"`
	ShiftID         *uint         `gorm:"index" json:"shift_id"`
	Shift           *Shift        `gorm:"foreignKey:ShiftID" json:"shift,omitempty"`
	Detail          string        `gorm:"size:1000" json:"detail"`
	IPAddress       string        `gorm:"size:50" json:"ip_address"`
	UserAgent       string        `gorm:"size:500" json:"user_agent"`
	CreatedAt       time.Time     `json:"created_at"`
}
