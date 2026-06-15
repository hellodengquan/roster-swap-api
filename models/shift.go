package models

import (
	"time"
)

type ShiftStatus string

const (
	ShiftStatusActive   ShiftStatus = "active"
	ShiftStatusCanceled ShiftStatus = "canceled"
	ShiftStatusSwapped  ShiftStatus = "swapped"
)

type Shift struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserID     uint       `gorm:"not null;index" json:"user_id"`
	User       User       `gorm:"foreignKey:UserID" json:"user,omitempty"`
	ShiftDate  time.Time  `gorm:"not null;index" json:"shift_date"`
	StartTime  string     `gorm:"size:10;not null" json:"start_time"`
	EndTime    string     `gorm:"size:10;not null" json:"end_time"`
	ShiftType  string     `gorm:"size:50" json:"shift_type"`
	Status     ShiftStatus `gorm:"size:20;default:'active'" json:"status"`
	Location   string     `gorm:"size:100" json:"location"`
	Note       string     `gorm:"size:500" json:"note"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
