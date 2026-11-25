package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func monitorError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, service.ErrMonitorConflict) {
		status = http.StatusConflict
	}
	common.ApiErrorStatus(c, status, common.NewMessage(err.Error()))
}

func monitorBind(c *gin.Context, input any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 12<<20)
	if err := common.DecodeJsonBodyStrict(c.Request.Body, input); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid monitor request"))
		return false
	}
	return true
}

func GetUpstreamMonitorState(c *gin.Context) {
	state, err := service.MonitorState()
	if err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, state)
}

func GetUpstreamMonitorBrowserSetup(c *gin.Context) {
	common.ApiSuccess(c, map[string]any{"browser_setup": service.MonitorBrowserSetupState()})
}

func GetUpstreamMonitorProfitDetail(c *gin.Context) {
	var selection service.MonitorProfitSelection
	if err := c.ShouldBindQuery(&selection); err != nil {
		monitorError(c, errors.New("Invalid monitor request"))
		return
	}
	detail, err := service.GetMonitorProfitDetail(selection)
	if err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func PutUpstreamMonitorConfig(c *gin.Context) {
	var config service.MonitorConfig
	if !monitorBind(c, &config) {
		return
	}
	result, err := service.SaveMonitorConfig(config)
	if err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func PairUpstreamMonitorDevice(c *gin.Context) {
	var input struct {
		Name string `json:"name"`
	}
	if !monitorBind(c, &input) {
		return
	}
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeMonitorDevicePair, Context: []byte("{}")}) == nil {
		return
	}
	result, err := service.CreateMonitorPair(c.GetInt("id"), input.Name)
	if err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func DeleteUpstreamMonitorDevice(c *gin.Context) {
	if err := service.RevokeMonitorDevice(c.Param("id")); err != nil {
		monitorError(c, err)
		return
	}
	service.RecordMonitorBrowserSetup(c.Param("id"), nil)
	common.ApiSuccess(c, nil)
}

func DeleteUpstreamMonitorDeviceRecord(c *gin.Context) {
	if err := service.DeleteMonitorDeviceRecord(c.Param("id")); err != nil {
		monitorError(c, err)
		return
	}
	service.RecordMonitorBrowserSetup(c.Param("id"), nil)
	common.ApiSuccess(c, nil)
}

func CreateUpstreamMonitorJob(c *gin.Context) {
	var input service.MonitorJobRequest
	if !monitorBind(c, &input) {
		return
	}
	job, err := service.CreateMonitorJob(input)
	if err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, job)
}

func GetUpstreamMonitorJob(c *gin.Context) {
	result, err := service.MonitorJobDetail(c.Param("id"))
	if err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func ControlUpstreamMonitorJob(c *gin.Context) {
	var input struct {
		Action string `json:"action"`
	}
	if !monitorBind(c, &input) {
		return
	}
	job, err := service.ControlMonitorJob(c.Param("id"), input.Action)
	if err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, job)
}

func UpstreamMonitorDeviceAuth(c *gin.Context) {
	raw := strings.TrimSpace(c.GetHeader("Authorization"))
	if !strings.HasPrefix(raw, "Bearer ") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Device authentication required"})
		return
	}
	device, err := service.AuthenticateMonitorDevice(strings.TrimSpace(strings.TrimPrefix(raw, "Bearer ")))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Device authentication failed"})
		return
	}
	c.Set("upstream_monitor_device", device)
	c.Next()
}

func BridgePairUpstreamMonitor(c *gin.Context) {
	var input struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if !monitorBind(c, &input) {
		return
	}
	result, err := service.CompleteMonitorPair(input.Code, input.Name)
	if err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func BridgeRestartUpstreamMonitor(c *gin.Context) {
	var input *struct {
		JobIDs common.RawMessage `json:"job_ids"`
	}
	if !monitorBind(c, &input) {
		return
	}
	if input == nil {
		monitorError(c, errors.New("Invalid monitor request"))
		return
	}
	var jobIDs []string
	if len(input.JobIDs) > 0 {
		if common.GetJsonType(input.JobIDs) != "array" || common.Unmarshal(input.JobIDs, &jobIDs) != nil {
			monitorError(c, errors.New("Invalid monitor request"))
			return
		}
	}
	device := c.MustGet("upstream_monitor_device").(*model.MonitorDevice)
	result, err := service.RestartMonitorDevice(device, jobIDs)
	if err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func BridgePollUpstreamMonitor(c *gin.Context) {
	var input struct {
		JobID      string                       `json:"job_id"`
		LeaseToken string                       `json:"lease_token"`
		Setup      *service.MonitorBrowserSetup `json:"setup"`
	}
	if !monitorBind(c, &input) {
		return
	}
	device := c.MustGet("upstream_monitor_device").(*model.MonitorDevice)
	result, err := service.PollMonitorDevice(device, input.JobID, input.LeaseToken)
	if err != nil {
		monitorError(c, err)
		return
	}
	service.RecordMonitorBrowserSetup(device.ID, input.Setup)
	common.ApiSuccess(c, result)
}

func BridgeResultUpstreamMonitor(c *gin.Context) {
	var input service.MonitorResultRequest
	if !monitorBind(c, &input) {
		return
	}
	device := c.MustGet("upstream_monitor_device").(*model.MonitorDevice)
	result, err := service.SubmitMonitorBrowserResult(device, c.Param("id"), input)
	if err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func BridgeSaveUpstreamMonitorFlow(c *gin.Context) {
	var input service.MonitorFlowSaveRequest
	if !monitorBind(c, &input) {
		return
	}
	device := c.MustGet("upstream_monitor_device").(*model.MonitorDevice)
	flow, err := service.SaveMonitorBrowserFlow(device, c.Param("id"), input)
	if err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, flow)
}

func BridgeEventUpstreamMonitor(c *gin.Context) {
	var event service.MonitorJobEvent
	if !monitorBind(c, &event) {
		return
	}
	device := c.MustGet("upstream_monitor_device").(*model.MonitorDevice)
	if err := service.MonitorRecordEvent(device, c.Param("id"), event); err != nil {
		monitorError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
