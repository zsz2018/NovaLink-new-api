package model

import (
	"errors"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const MonitorBrowserRestartMessage = "Browser executor restarted; interrupted task was stopped"

// RestartMonitorBrowserJobs discovers active executions when jobIDs is nil, or
// interrupts only the supplied executions. Exact IDs make delayed confirmations
// harmless after a subsequent poll has assigned a new task.
func RestartMonitorBrowserJobs(authenticated *MonitorDevice, jobIDs []string, now int64) ([]string, int64, error) {
	activeIDs := []string{}
	var interrupted int64
	if authenticated == nil || authenticated.TokenHash == "" {
		return nil, 0, errors.New("Invalid device token")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var device MonitorDevice
		if err := lockForUpdate(tx).Where("id = ? AND owner_id = ? AND token_hash = ? AND revoked_at = 0 AND expires_at > ?", authenticated.ID, authenticated.OwnerID, authenticated.TokenHash, now).First(&device).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("Invalid device token")
			}
			return err
		}
		var owner User
		if err := lockForUpdate(tx).Select("id", "role", "status").First(&owner, device.OwnerID).Error; err != nil || owner.Role != common.RoleRootUser || owner.Status != common.UserStatusEnabled {
			return errors.New("Monitor device owner is no longer authorized")
		}
		query := tx.Model(&MonitorJob{}).Where("device_id = ? AND status IN ?", device.ID, []string{"running", "awaiting_user"})
		if jobIDs != nil {
			if len(jobIDs) == 0 {
				return nil
			}
			query = query.Where("id IN ?", jobIDs)
		}
		if err := query.Order("created_at, id").Limit(101).Pluck("id", &activeIDs).Error; err != nil {
			return err
		}
		if len(activeIDs) > 100 {
			return errors.New("Invalid monitor request")
		}
		if jobIDs == nil || len(activeIDs) == 0 {
			return nil
		}
		updated := tx.Model(&MonitorJob{}).Where("id IN ? AND device_id = ? AND status IN ?", activeIDs, device.ID, []string{"running", "awaiting_user"}).Updates(map[string]any{
			"status": "failed", "message": MonitorBrowserRestartMessage, "control": "cancel", "complete": false,
			"lease_hash": "", "lease_until": 0, "frozen": "", "updated_at": now,
		})
		if updated.Error != nil {
			return updated.Error
		}
		interrupted = updated.RowsAffected
		if slices.Contains(activeIDs, device.ActiveJob) {
			return tx.Model(&MonitorDevice{}).Where("id = ? AND active_job = ?", device.ID, device.ActiveJob).Update("active_job", "").Error
		}
		return nil
	})
	return activeIDs, interrupted, err
}

// ClaimMonitorBrowserJob serializes task assignment on the device row. The
// device slot and job lease become visible together, so concurrent polls cannot
// start two logins against one browser profile.
func ClaimMonitorBrowserJob(deviceID, leaseHash string, now, until int64) (*MonitorJob, error) {
	var claimed *MonitorJob
	err := DB.Transaction(func(tx *gorm.DB) error {
		var device MonitorDevice
		if err := lockForUpdate(tx).Where("id = ? AND revoked_at = 0 AND expires_at > ?", deviceID, now).First(&device).Error; err != nil {
			return err
		}
		if device.ActiveJob != "" {
			var active MonitorJob
			err := tx.First(&active, "id = ?", device.ActiveJob).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil && (active.Status == "running" || active.Status == "awaiting_user") && active.LeaseUntil >= now {
				return nil
			}
		}
		var job MonitorJob
		if err := tx.Where("device_id = ? AND status = ?", deviceID, "queued").Order("created_at, id").First(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		result := tx.Model(&MonitorDevice{}).Where("id = ? AND active_job = ? AND revoked_at = 0", deviceID, device.ActiveJob).Update("active_job", job.ID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		result = tx.Model(&MonitorJob{}).Where("id = ? AND status = ?", job.ID, "queued").Updates(map[string]any{"status": "running", "lease_hash": leaseHash, "lease_until": until, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("Monitor task changed during assignment")
		}
		job.Status, job.LeaseHash, job.LeaseUntil, job.UpdatedAt = "running", leaseHash, until, now
		claimed = &job
		return nil
	})
	return claimed, err
}
