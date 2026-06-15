package models

import (
	"time"
)

type DelegateStatus string

const (
	DelegateStatusActive   DelegateStatus = "active"
	DelegateStatusExpired  DelegateStatus = "expired"
	DelegateStatusRevoked DelegateStatus = "revoked"
)

type ApproverDelegate struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	DelegatorID    uint           `gorm:"not null;index" json:"delegator_id"`
	Delegator      User          `gorm:"foreignKey:DelegatorID" json:"delegator,omitempty"`
	DelegateID     uint           `gorm:"not null;index" json:"delegate_id"`
	Delegate        User          `gorm:"foreignKey:DelegateID" json:"delegate,omitempty"`
	StartDate     time.Time      `gorm:"not null;index" json:"start_date"`
	EndDate       time.Time      `gorm:"not null;index" json:"end_date"`
	Reason        string         `gorm:"size:500" json:"reason"`
	Status        DelegateStatus `gorm:"size:20;default:'active'" json:"status"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func (d *ApproverDelegate) IsActive() bool {
	if d.Status != DelegateStatusActive {
		return false
	}
	now := time.Now()
	return !now.Before(d.StartDate) && !now.After(d.EndDate)
}
