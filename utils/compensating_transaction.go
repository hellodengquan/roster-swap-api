package utils

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type TxStepStatus string

const (
	TxStepPending   TxStepStatus = "pending"
	TxStepRunning   TxStepStatus = "running"
	TxStepCompleted TxStepStatus = "completed"
	TxStepFailed    TxStepStatus = "failed"
	TxStepCompensating TxStepStatus = "compensating"
	TxStepCompensated  TxStepStatus = "compensated"
	TxStepCompensateFailed TxStepStatus = "compensate_failed"
)

type TxStep struct {
	ID         string
	Name       string
	Status     TxStepStatus
	Execute    func(tx *gorm.DB) error
	Compensate func(tx *gorm.DB) error
	Error      error
	StartedAt  *time.Time
	FinishedAt *time.Time
}

type CompensatingTransaction struct {
	ID        string
	Steps     []*TxStep
	DB        *gorm.DB
	Status    string
	Error     error
	StartedAt time.Time
	EndedAt   *time.Time
}

func NewCompensatingTransaction(db *gorm.DB, id string) *CompensatingTransaction {
	return &CompensatingTransaction{
		ID:        id,
		DB:        db,
		Steps:     []*TxStep{},
		Status:    "initialized",
		StartedAt: time.Now(),
	}
}

func (ctx *CompensatingTransaction) AddStep(id, name string, execute, compensate func(tx *gorm.DB) error) {
	ctx.Steps = append(ctx.Steps, &TxStep{
		ID:         id,
		Name:       name,
		Status:     TxStepPending,
		Execute:    execute,
		Compensate: compensate,
	})
}

func (s *TxStep) markStarted() {
	now := time.Now()
	s.StartedAt = &now
	s.Status = TxStepRunning
}

func (s *TxStep) markCompleted() {
	now := time.Now()
	s.FinishedAt = &now
	s.Status = TxStepCompleted
}

func (s *TxStep) markFailed(err error) {
	now := time.Now()
	s.FinishedAt = &now
	s.Status = TxStepFailed
	s.Error = err
}

func (s *TxStep) markCompensating() {
	s.Status = TxStepCompensating
}

func (s *TxStep) markCompensated() {
	s.Status = TxStepCompensated
}

func (s *TxStep) markCompensateFailed(err error) {
	s.Status = TxStepCompensateFailed
	s.Error = err
}

func (ctx *CompensatingTransaction) Execute() error {
	ctx.Status = "running"

	for i, step := range ctx.Steps {
		step.markStarted()

		err := step.Execute(ctx.DB)
		if err != nil {
			step.markFailed(err)
			ctx.Error = err
			ctx.Status = "failed"

			compensateErr := ctx.compensate(i)
			if compensateErr != nil {
				ctx.Error = fmt.Errorf("执行失败: %v; 补偿失败: %v", err, compensateErr)
				ctx.Status = "compensate_failed"
			} else {
				ctx.Status = "compensated"
			}

			now := time.Now()
			ctx.EndedAt = &now
			return ctx.Error
		}

		step.markCompleted()
	}

	ctx.Status = "completed"
	now := time.Now()
	ctx.EndedAt = &now
	return nil
}

func (ctx *CompensatingTransaction) compensate(failedStepIndex int) error {
	var firstErr error

	for i := failedStepIndex - 1; i >= 0; i-- {
		step := ctx.Steps[i]
		if step.Status != TxStepCompleted {
			continue
		}

		step.markCompensating()

		err := step.Compensate(ctx.DB)
		if err != nil {
			step.markCompensateFailed(err)
			if firstErr == nil {
				firstErr = fmt.Errorf("步骤 %s 补偿失败: %v", step.Name, err)
			}
		} else {
			step.markCompensated()
		}
	}

	return firstErr
}

func (ctx *CompensatingTransaction) GetSummary() map[string]interface{} {
	stepsSummary := make([]map[string]interface{}, 0, len(ctx.Steps))
	for _, s := range ctx.Steps {
		errMsg := ""
		if s.Error != nil {
			errMsg = s.Error.Error()
		}
		stepsSummary = append(stepsSummary, map[string]interface{}{
			"id":         s.ID,
			"name":       s.Name,
			"status":     s.Status,
			"error":      errMsg,
			"started_at": s.StartedAt,
			"finished_at": s.FinishedAt,
		})
	}

	return map[string]interface{}{
		"tx_id":     ctx.ID,
		"status":    ctx.Status,
		"error":     ctx.Error,
		"started_at": ctx.StartedAt,
		"ended_at":  ctx.EndedAt,
		"steps":     stepsSummary,
		"step_count": len(ctx.Steps),
	}
}

func (ctx *CompensatingTransaction) IsSuccess() bool {
	return ctx.Status == "completed"
}

func (ctx *CompensatingTransaction) IsCompensated() bool {
	return ctx.Status == "compensated"
}
