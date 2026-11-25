package service

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const monitorLeaseSeconds = 120

func CreateMonitorPair(owner int, name string) (map[string]any, error) {
	if !MonitorKeyReady() {
		return nil, errors.New("Monitor encryption key is not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return nil, errors.New("Invalid browser device name")
	}
	code := monitorID("")
	now := common.GetTimestamp()
	device := model.MonitorDevice{ID: monitorID("dev_"), OwnerID: owner, Name: name, PairHash: monitorHash(code), PairExpires: now + 300, CreatedAt: now}
	if err := model.DB.Create(&device).Error; err != nil {
		return nil, err
	}
	return map[string]any{"device_id": device.ID, "code": code, "expires_at": device.PairExpires}, nil
}

func CompleteMonitorPair(code, name string) (map[string]string, error) {
	if len(code) != 48 {
		return nil, errors.New("Invalid or expired pairing code")
	}
	var device model.MonitorDevice
	now := common.GetTimestamp()
	if err := model.DB.Where("pair_hash = ? AND pair_expires > ? AND revoked_at = 0", monitorHash(code), now).First(&device).Error; err != nil {
		return nil, errors.New("Invalid or expired pairing code")
	}
	if err := monitorOwner(device.OwnerID); err != nil {
		return nil, err
	}
	token := monitorID("bridge_")
	updates := map[string]any{"pair_hash": "", "pair_expires": 0, "token_hash": monitorHash(token), "expires_at": now + 180*86400, "last_seen_at": now}
	if strings.TrimSpace(name) != "" && len(name) <= 128 {
		updates["name"] = strings.TrimSpace(name)
	}
	result := model.DB.Model(&model.MonitorDevice{}).Where("id = ? AND pair_hash = ? AND pair_expires > ? AND revoked_at = 0", device.ID, monitorHash(code), now).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, errors.New("Invalid or expired pairing code")
	}
	return map[string]string{"device_id": device.ID, "token": token}, nil
}

func monitorOwner(owner int) error {
	var user model.User
	if err := model.DB.Select("id", "role", "status").First(&user, owner).Error; err != nil || user.Role != common.RoleRootUser || user.Status != common.UserStatusEnabled {
		return errors.New("Monitor device owner is no longer authorized")
	}
	return nil
}

func AuthenticateMonitorDevice(token string) (*model.MonitorDevice, error) {
	if !strings.HasPrefix(token, "bridge_") || len(token) != 55 {
		return nil, errors.New("Invalid device token")
	}
	var device model.MonitorDevice
	if err := model.DB.Where("token_hash = ? AND expires_at > ? AND revoked_at = 0", monitorHash(token), common.GetTimestamp()).First(&device).Error; err != nil {
		return nil, errors.New("Invalid device token")
	}
	if err := monitorOwner(device.OwnerID); err != nil {
		return nil, err
	}
	return &device, nil
}

// RestartMonitorDevice separates read-only discovery from confirmation so a
// retried startup request can never interrupt a newly assigned execution.
func RestartMonitorDevice(device *model.MonitorDevice, jobIDs []string) (map[string]any, error) {
	if len(jobIDs) > 100 {
		return nil, errors.New("Invalid monitor request")
	}
	seen := make(map[string]struct{}, len(jobIDs))
	for _, id := range jobIDs {
		if len(id) == 0 || len(id) > 64 {
			return nil, errors.New("Invalid monitor request")
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, errors.New("Invalid monitor request")
		}
		seen[id] = struct{}{}
	}
	activeIDs, interrupted, err := model.RestartMonitorBrowserJobs(device, jobIDs, common.GetTimestamp())
	if err != nil {
		return nil, err
	}
	if jobIDs == nil {
		return map[string]any{"active_job_ids": activeIDs}, nil
	}
	return map[string]any{"interrupted": interrupted}, nil
}

func RevokeMonitorDevice(id string) error {
	now := common.GetTimestamp()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.MonitorDevice{}).Where("id = ?", id).Updates(map[string]any{"revoked_at": now, "token_hash": "", "pair_hash": ""}).Error; err != nil {
			return err
		}
		return tx.Model(&model.MonitorJob{}).Where("device_id = ? AND status IN ?", id, []string{"queued", "running", "awaiting_user"}).Updates(map[string]any{"status": "cancelled", "control": "cancel", "lease_hash": "", "frozen": "", "updated_at": now}).Error
	})
}

func DeleteMonitorDeviceRecord(id string) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var device model.MonitorDevice
		if err := tx.First(&device, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if device.RevokedAt == 0 {
			return errors.New("Revoke the browser before deleting its record")
		}
		var stored model.MonitorConfiguration
		if err := tx.First(&stored, "id = 1").Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if stored.ID != 0 {
			var config MonitorConfig
			if err := common.UnmarshalJsonStr(string(stored.Content), &config); err != nil {
				return err
			}
			changed := false
			for i := range config.Accounts {
				if config.Accounts[i].DeviceID == id {
					config.Accounts[i].DeviceID = ""
					config.Accounts[i].IntervalMinutes = 0
					changed = true
				}
			}
			if changed {
				config.Version = stored.Version + 1
				encoded, err := common.Marshal(config)
				if err != nil {
					return err
				}
				updated := tx.Model(&model.MonitorConfiguration{}).Where("id = 1 AND version = ?", stored.Version).Updates(map[string]any{"version": config.Version, "content": model.LongText(encoded)})
				if updated.Error != nil {
					return updated.Error
				}
				if updated.RowsAffected != 1 {
					return ErrMonitorConflict
				}
			}
		}
		return tx.Where("id = ? AND revoked_at > 0", id).Delete(&model.MonitorDevice{}).Error
	})
}

func CreateMonitorJob(request MonitorJobRequest) (*model.MonitorJob, error) {
	if !slices.Contains([]string{"collect", "record"}, request.Mode) {
		return nil, errors.New("Invalid monitor job mode")
	}
	config, err := GetMonitorConfig()
	if err != nil {
		return nil, err
	}
	var account MonitorAccount
	for _, candidate := range config.Accounts {
		if candidate.ID == request.AccountID {
			account = candidate
			break
		}
	}
	if account.ID == "" {
		return nil, errors.New("Monitor account not found")
	}
	if request.DeviceID == "" {
		request.DeviceID = account.DeviceID
	}
	if request.FlowID == "" && request.Mode == "collect" {
		request.FlowID = account.FlowID
	}
	var device model.MonitorDevice
	if err := model.DB.Where("id = ? AND revoked_at = 0 AND expires_at > ?", request.DeviceID, common.GetTimestamp()).First(&device).Error; err != nil {
		return nil, errors.New("Paired browser device not found")
	}
	if err := monitorOwner(device.OwnerID); err != nil {
		return nil, err
	}
	var site MonitorSite
	for _, candidate := range config.Sites {
		if candidate.ID == account.SiteID {
			site = candidate
			break
		}
	}
	var flow MonitorFlow
	for _, candidate := range config.Flows {
		if candidate.ID == request.FlowID && candidate.SiteID == site.ID {
			flow = candidate
			break
		}
	}
	if flow.ID == "" {
		if request.Mode != "record" || request.FlowID != "" {
			return nil, errors.New("Monitor flow not found")
		}
		flow = MonitorFlow{ID: monitorID("flow_"), SiteID: site.ID, Name: account.Name, Steps: []MonitorStep{}, CaptureSeconds: 15}
	}
	if request.Mode == "collect" {
		if flow.Extraction == nil || account.ExternalID == "" {
			return nil, errors.New("Map account identity, balances and groups before collecting")
		}
		if err := validateMonitorExtraction(flow.Extraction, site); err != nil {
			return nil, err
		}
	}
	secrets, err := monitorSecrets()
	if err != nil {
		return nil, err
	}
	account.Password = secrets["password:"+account.ID]
	account.ClearPassword = false
	account.OfficialEstimate = nil // Comparison settings never enter browser tasks, including their frozen inputs.
	execution := MonitorExecution{Mode: request.Mode, Site: site, Account: account, Flow: flow}
	encoded, err := common.Marshal(execution)
	if err != nil {
		return nil, err
	}
	frozen, err := monitorEncrypt(string(encoded))
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	job := &model.MonitorJob{ID: monitorID("job_"), AccountID: account.ID, DeviceID: device.ID, FlowID: flow.ID, Mode: request.Mode, Status: "queued", ConfigVersion: config.Version, FlowRevision: flow.Revision, Frozen: model.LongText(frozen), CreatedAt: now, UpdatedAt: now}
	if err := model.DB.Create(job).Error; err != nil {
		return nil, err
	}
	return job, nil
}

func monitorExecution(job *model.MonitorJob) (MonitorExecution, error) {
	var execution MonitorExecution
	decoded, err := monitorDecrypt(string(job.Frozen))
	if err != nil {
		return execution, err
	}
	if err := common.DecodeJsonBodyStrict(strings.NewReader(decoded), &execution); err != nil {
		return execution, err
	}
	execution.ID = job.ID
	execution.Account.OfficialEstimate = nil
	return execution, nil
}

func PollMonitorDevice(device *model.MonitorDevice, jobID, leaseToken string) (map[string]any, error) {
	now := common.GetTimestamp()
	if err := model.DB.Model(device).Updates(map[string]any{"last_seen_at": now}).Error; err != nil {
		return nil, err
	}
	if err := model.DB.Model(&model.MonitorJob{}).Where("device_id = ? AND status IN ? AND lease_until > 0 AND lease_until < ?", device.ID, []string{"running", "awaiting_user"}, now).Updates(map[string]any{"status": "failed", "message": "Browser lease expired", "lease_hash": "", "updated_at": now}).Error; err != nil {
		return nil, err
	}
	if jobID != "" {
		var job model.MonitorJob
		if err := model.DB.Where("id = ? AND device_id = ?", jobID, device.ID).First(&job).Error; err != nil {
			return nil, errors.New("Monitor job not found")
		}
		if slices.Contains([]string{"cancelled", "failed", "completed"}, job.Status) {
			return map[string]any{"job": nil, "control": "cancel", "status": job.Status}, nil
		}
		if leaseToken == "" || job.LeaseHash != monitorHash(leaseToken) || job.LeaseUntil < now {
			return nil, errors.New("Invalid monitor execution lease")
		}
		result := model.DB.Model(&model.MonitorJob{}).Where("id = ? AND lease_hash = ? AND status IN ?", job.ID, job.LeaseHash, []string{"running", "awaiting_user"}).Updates(map[string]any{"lease_until": now + monitorLeaseSeconds, "updated_at": now})
		if result.Error != nil {
			return nil, result.Error
		}
		return map[string]any{"job": nil, "control": job.Control, "status": job.Status}, nil
	}
	lease := monitorID("")
	job, err := model.ClaimMonitorBrowserJob(device.ID, monitorHash(lease), now, now+monitorLeaseSeconds)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return map[string]any{"job": nil}, nil
	}
	execution, err := monitorExecution(job)
	if err != nil {
		model.DB.Model(job).Updates(map[string]any{"status": "failed", "message": "Unable to load execution credentials"})
		return nil, err
	}
	execution.LeaseToken = lease
	return map[string]any{"job": execution}, nil
}

func ControlMonitorJob(id, action string) (*model.MonitorJob, error) {
	var job model.MonitorJob
	if err := model.DB.First(&job, "id = ?", id).Error; err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	switch action {
	case "cancel":
		if err := model.DB.Model(&job).Where("status IN ?", []string{"queued", "running", "awaiting_user"}).Updates(map[string]any{"status": "cancelled", "control": "cancel", "lease_hash": "", "updated_at": now}).Error; err != nil {
			return nil, err
		}
	case "resume":
		if job.Status != "awaiting_user" {
			return nil, errors.New("Monitor job is not waiting for a user")
		}
		if err := model.DB.Model(&job).Updates(map[string]any{"status": "running", "control": "resume", "updated_at": now}).Error; err != nil {
			return nil, err
		}
	case "retry":
		flowID := job.FlowID
		if job.Mode == "record" && job.FlowRevision == 0 {
			flowID = ""
		}
		return CreateMonitorJob(MonitorJobRequest{AccountID: job.AccountID, DeviceID: job.DeviceID, FlowID: flowID, Mode: job.Mode})
	default:
		return nil, errors.New("Invalid monitor control action")
	}
	return &job, nil
}

func MonitorJobDetail(id string) (map[string]any, error) {
	var job model.MonitorJob
	if err := model.DB.First(&job, "id = ?", id).Error; err != nil {
		return nil, err
	}
	steps := []MonitorStep{}
	if job.RecordedSteps != "" {
		if err := common.UnmarshalJsonStr(string(job.RecordedSteps), &steps); err != nil {
			return nil, err
		}
	}
	result := map[string]any{"job": job, "recorded_steps": steps}
	var snapshot model.MonitorSnapshot
	if err := model.DB.Where("job_id = ?", id).Order("created_at desc, id desc").First(&snapshot).Error; err == nil {
		result["snapshot"] = monitorSnapshotResponse(snapshot)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return result, nil
}

func MonitorRecordEvent(device *model.MonitorDevice, id string, event MonitorJobEvent) error {
	if !slices.Contains([]string{"progress", "recording", "complete", "failed", "awaiting_user"}, event.Type) {
		return errors.New("Invalid monitor event")
	}
	if len(event.Message) > 2000 || event.Step < 0 || event.Step > 2000 {
		return errors.New("Invalid monitor event limits")
	}
	var job model.MonitorJob
	now := common.GetTimestamp()
	if err := model.DB.Where("id = ? AND device_id = ? AND status IN ? AND lease_hash = ? AND lease_until >= ?", id, device.ID, []string{"running", "awaiting_user"}, monitorHash(event.LeaseToken), now).First(&job).Error; err != nil {
		return errors.New("Invalid or expired monitor execution lease")
	}
	execution, err := monitorExecution(&job)
	if err != nil {
		return err
	}
	if job.Mode == "collect" && (event.Type == "complete" || len(event.Steps) != 0) {
		return errors.New("Submit the combined structured result to finish collecting")
	}
	if len(event.Steps) > 0 {
		recorded := MonitorFlow{Steps: event.Steps, Extraction: execution.Flow.Extraction}
		if err := validateMonitorFlowSteps(recorded, execution.Site); err != nil {
			return err
		}
		recorded.Extraction = nil
		if err := validateMonitorFlowSecrets(recorded, []string{execution.Account.Password}); err != nil {
			return err
		}
	}
	if execution.Account.Password != "" {
		event.Message = strings.ReplaceAll(event.Message, execution.Account.Password, "[redacted]")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := monitorBrowserOwner(tx, device.ID, now); err != nil {
			return err
		}
		updates := map[string]any{"step": event.Step, "message": event.Message, "updated_at": now, "lease_until": now + monitorLeaseSeconds}
		if len(event.Steps) > 0 {
			encoded, err := common.Marshal(event.Steps)
			if err != nil {
				return err
			}
			updates["recorded_steps"] = string(encoded)
		}
		if event.Type == "awaiting_user" {
			updates["status"] = "awaiting_user"
			updates["control"] = ""
		}
		if event.Type == "failed" {
			updates["status"] = "failed"
			updates["complete"] = false
			updates["lease_hash"] = ""
		}
		if event.Type == "complete" {
			updates["status"] = "completed"
			updates["complete"] = event.Complete
			updates["lease_hash"] = ""
			updates["control"] = ""
		}
		result := tx.Model(&model.MonitorJob{}).Where("id = ? AND device_id = ? AND status IN ? AND lease_hash = ? AND lease_until >= ?", id, device.ID, []string{"running", "awaiting_user"}, monitorHash(event.LeaseToken), now).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			// MySQL reports changed rows; repeating a progress event in the same
			// second can be a no-op while this execution still owns its lease.
			var held int64
			if err := tx.Model(&model.MonitorJob{}).Where("id = ? AND device_id = ? AND status IN ? AND lease_hash = ? AND lease_until >= ?", id, device.ID, []string{"running", "awaiting_user"}, monitorHash(event.LeaseToken), now).Count(&held).Error; err != nil {
				return err
			}
			if held != 1 {
				return errors.New("Monitor execution lease changed")
			}
		}
		return nil
	})
}

func monitorSnapshotResponse(snapshot model.MonitorSnapshot) map[string]any {
	var data MonitorData
	_ = common.UnmarshalJsonStr(string(snapshot.Data), &data)
	return map[string]any{"id": snapshot.ID, "job_id": snapshot.JobID, "account_id": snapshot.AccountID, "data": data, "confirmed": snapshot.Confirmed, "complete": snapshot.Complete, "created_at": snapshot.CreatedAt, "confirmed_at": snapshot.ConfirmedAt}
}

func monitorDeviceIntervalDue(last int64, minutes int) bool {
	return minutes > 0 && common.GetTimestamp()-last >= int64(time.Duration(minutes)*time.Minute/time.Second)
}
