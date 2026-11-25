package service

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// SaveMonitorBrowserFlow grants an active browser recording only the right to
// save its assigned flow. Other configuration and encrypted secrets stay intact.
func SaveMonitorBrowserFlow(device *model.MonitorDevice, id string, request MonitorFlowSaveRequest) (MonitorFlow, error) {
	flow := request.Flow
	now := common.GetTimestamp()
	var job model.MonitorJob
	if request.LeaseToken == "" {
		return flow, errors.New("Invalid or expired monitor execution lease")
	}
	if err := model.DB.Where("id = ? AND device_id = ? AND mode = ? AND status IN ? AND lease_hash = ? AND lease_until >= ?", id, device.ID, "record", []string{"running", "awaiting_user"}, monitorHash(request.LeaseToken), now).First(&job).Error; err != nil {
		return flow, errors.New("Invalid or expired monitor execution lease")
	}
	execution, err := monitorExecution(&job)
	if err != nil {
		return flow, err
	}
	if flow.ID != execution.Flow.ID || flow.SiteID != execution.Site.ID {
		return flow, errors.New("Monitor flow does not belong to this browser task")
	}
	if flow.Revision != job.FlowRevision {
		return flow, ErrMonitorConflict
	}
	if err := validateMonitorFlowSteps(flow, execution.Site); err != nil {
		return flow, err
	}
	flow.Name = strings.TrimSpace(flow.Name)
	if flow.Name == "" || len(flow.Name) > 128 || flow.CaptureSeconds < 1 {
		return flow, errors.New("Invalid monitor flow")
	}
	config, err := GetMonitorConfig()
	if err != nil {
		return flow, err
	}
	index := -1
	for i, candidate := range config.Flows {
		if candidate.ID == flow.ID {
			if candidate.SiteID != flow.SiteID || candidate.Revision != flow.Revision {
				return flow, ErrMonitorConflict
			}
			index = i
			break
		}
	}
	if index < 0 && flow.Revision != 0 {
		return flow, ErrMonitorConflict
	}
	accountIndex := slices.IndexFunc(config.Accounts, func(account MonitorAccount) bool { return account.ID == job.AccountID })
	if accountIndex < 0 {
		return flow, errors.New("Monitor account not found")
	}
	if config.Accounts[accountIndex].SiteID != execution.Site.ID {
		return flow, ErrMonitorConflict
	}
	// Previously verified mappings remain valid when only page actions or login
	// configuration change. Compare before normalizing the request's page URLs,
	// and require agreement with both the frozen task and current configuration.
	retainVerifiedMappings := index >= 0 && job.FlowRevision > 0 && execution.Flow.Revision == job.FlowRevision &&
		flow.Extraction != nil && reflect.DeepEqual(flow.Extraction, execution.Flow.Extraction) &&
		reflect.DeepEqual(flow.Extraction, config.Flows[index].Extraction) &&
		config.Accounts[accountIndex].ExternalID != "" &&
		config.Accounts[accountIndex].ExternalID == execution.Account.ExternalID
	if (request.Preview == nil && !retainVerifiedMappings) || (request.Preview != nil && !request.Preview.Complete) {
		return flow, errors.New("Preview the complete structured result before saving mappings")
	}
	if flow.Extraction == nil {
		return flow, errors.New("Map account identity, balances and groups before collecting")
	}
	for i := range flow.Extraction.Requests {
		rule := &flow.Extraction.Requests[i]
		if len(rule.PageURLs) == 0 {
			return flow, errors.New("Associate each selected request with its source page before saving")
		}
		for j, raw := range rule.PageURLs {
			page, err := monitorCollectionPage(raw, execution.Site)
			if err != nil {
				return flow, err
			}
			rule.PageURLs[j] = page
		}
	}
	execution.Flow = flow
	if request.Preview != nil {
		preview, err := validateMonitorBatch(*request.Preview, execution, job.CreatedAt)
		if err != nil {
			return flow, err
		}
		if expected := config.Accounts[accountIndex].ExternalID; expected != "" && expected != preview.AccountID {
			return flow, errors.New("Captured account identity does not match the configured account")
		}
		config.Accounts[accountIndex].ExternalID = preview.AccountID
		execution.Account.ExternalID = preview.AccountID
	}
	secrets, err := monitorSecrets()
	if err != nil {
		return flow, err
	}
	// Include the frozen credential when an account password changed during the
	// task. Neither the current nor the old password may become workflow text.
	secrets["task_password"] = execution.Account.Password
	if err := validateMonitorFlowSecrets(flow, slices.Collect(maps.Values(secrets))); err != nil {
		return flow, err
	}
	flow.Revision++
	if index < 0 {
		config.Flows = append(config.Flows, flow)
	} else {
		config.Flows[index] = flow
	}
	sites, err := validateMonitorConfigStructure(config)
	if err != nil {
		return flow, err
	}
	// An unchanged flow may need an explicit rebuild of its obsolete mappings.
	// Validate this recording against the current site without rewriting or
	// admitting the unrelated extraction rules for execution.
	if err := validateMonitorExtraction(flow.Extraction, sites[flow.SiteID]); err != nil {
		return flow, err
	}
	expectedVersion := config.Version
	config.Version++
	encoded, err := common.Marshal(config)
	if err != nil {
		return flow, err
	}
	if len(encoded) > monitorMaxConfigBytes {
		return flow, errors.New("Monitor configuration is too large")
	}
	execution.Flow = flow
	execution.Account.OfficialEstimate = nil
	frozenJSON, err := common.Marshal(execution)
	if err != nil {
		return flow, err
	}
	frozen, err := monitorEncrypt(string(frozenJSON))
	if err != nil {
		return flow, err
	}
	steps, err := common.Marshal(flow.Steps)
	if err != nil {
		return flow, err
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := monitorBrowserOwner(tx, device.ID, now); err != nil {
			return err
		}
		if expectedVersion == 0 {
			if err := tx.Create(&model.MonitorConfiguration{ID: 1, Version: config.Version, Content: model.LongText(encoded)}).Error; err != nil {
				return ErrMonitorConflict
			}
		} else {
			updated := tx.Model(&model.MonitorConfiguration{}).Where("id = 1 AND version = ?", expectedVersion).Updates(map[string]any{"version": config.Version, "content": model.LongText(encoded)})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return ErrMonitorConflict
			}
		}
		updated := tx.Model(&model.MonitorJob{}).Where("id = ? AND device_id = ? AND mode = ? AND status IN ? AND lease_hash = ? AND lease_until >= ? AND flow_revision = ?", id, device.ID, "record", []string{"running", "awaiting_user"}, monitorHash(request.LeaseToken), common.GetTimestamp(), job.FlowRevision).Updates(map[string]any{"flow_id": flow.ID, "flow_revision": flow.Revision, "config_version": config.Version, "frozen": frozen, "recorded_steps": model.LongText(steps), "updated_at": common.GetTimestamp()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("Monitor execution lease changed")
		}
		return nil
	})
	return flow, err
}
