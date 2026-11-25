package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type monitorSystemHandler struct{}

func (monitorSystemHandler) Type() string            { return "upstream_monitor" }
func (monitorSystemHandler) Enabled() bool           { return true }
func (monitorSystemHandler) Interval() time.Duration { return 30 * time.Second }
func (monitorSystemHandler) NewPayload() any         { return nil }
func (monitorSystemHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	err := runMonitorScheduler(ctx)
	status, message := model.SystemTaskStatusSucceeded, ""
	if err != nil {
		status, message = model.SystemTaskStatusFailed, err.Error()
	}
	_ = model.FinishSystemTask(task.TaskID, runnerID, status, nil, message)
}

func init() { RegisterSystemTaskHandler(monitorSystemHandler{}) }

func runMonitorScheduler(ctx context.Context) error {
	if err := model.DB.Model(&model.MonitorJob{}).Where("status IN ? AND lease_until > 0 AND lease_until < ?", []string{"running", "awaiting_user"}, common.GetTimestamp()).Updates(map[string]any{"status": "failed", "message": "Browser lease expired", "lease_hash": "", "updated_at": common.GetTimestamp()}).Error; err != nil {
		return err
	}
	config, err := GetMonitorConfig()
	if err != nil {
		return err
	}
	for _, account := range config.Accounts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !account.Enabled || account.IntervalMinutes <= 0 || account.DeviceID == "" || account.FlowID == "" {
			continue
		}
		var latest model.MonitorJob
		err := model.DB.Where("account_id = ?", account.ID).Order("created_at desc, id desc").First(&latest).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if slicesMonitorActive(latest.Status) || latest.Message == "Browser lease expired" || latest.Message == model.MonitorBrowserRestartMessage || !monitorDeviceIntervalDue(latest.CreatedAt, account.IntervalMinutes) {
			continue
		}
		_, _ = CreateMonitorJob(MonitorJobRequest{AccountID: account.ID, Mode: "collect"})
	}
	return nil
}

func slicesMonitorActive(status string) bool {
	return status == "queued" || status == "running" || status == "awaiting_user"
}
