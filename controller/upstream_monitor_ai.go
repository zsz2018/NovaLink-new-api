package controller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func monitorAIBind(c *gin.Context, input any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MonitorAIMaxRequestBytes)
	if err := common.DecodeJsonBodyStrict(c.Request.Body, input); err != nil {
		issue := service.MonitorAIValidationIssue{Code: "invalid_json"}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			issue.Code = "material_size"
		}
		monitorAIError(c, &service.MonitorAIValidationError{Issue: issue})
		return false
	}
	return true
}

func monitorAIError(c *gin.Context, err error) {
	var validation *service.MonitorAIValidationError
	if errors.As(err, &validation) {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage(service.ErrMonitorAIRequest.Error()), gin.H{"validation_issue": validation.Issue})
		return
	}
	var proposal *service.MonitorAIProposalError
	if errors.As(err, &proposal) {
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage(service.ErrMonitorAIProposal.Error()), gin.H{"proposal_issue": proposal.Issue})
		return
	}
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, service.ErrMonitorAIAuthorization):
		status = http.StatusForbidden
	case errors.Is(err, service.ErrMonitorAILimit):
		status = http.StatusTooManyRequests
	case errors.Is(err, service.ErrMonitorAIModel):
		status = http.StatusServiceUnavailable
	case errors.Is(err, service.ErrMonitorAIProposal):
		status = http.StatusBadGateway
	}
	common.ApiErrorStatus(c, status, common.NewMessage(err.Error()))
}

func BridgeMonitorAIModels(c *gin.Context) {
	var input service.MonitorAILeaseRequest
	if !monitorAIBind(c, &input) {
		return
	}
	device := c.MustGet("upstream_monitor_device").(*model.MonitorDevice)
	auth, err := service.AuthorizeMonitorAI(device, c.Param("id"), input.LeaseToken)
	if err != nil {
		monitorAIError(c, err)
		return
	}
	models, err := service.MonitorAIAvailableModels(auth.Owner)
	if err != nil {
		monitorAIError(c, err)
		return
	}
	if _, err = service.AuthorizeMonitorAI(device, c.Param("id"), input.LeaseToken); err != nil {
		monitorAIError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"models": models})
}

func BridgeGenerateMonitorAI(c *gin.Context) {
	var input service.MonitorAIGenerateRequest
	if !monitorAIBind(c, &input) {
		return
	}
	device := c.MustGet("upstream_monitor_device").(*model.MonitorDevice)
	auth, err := service.AuthorizeMonitorAI(device, c.Param("id"), input.LeaseToken)
	if err != nil {
		monitorAIError(c, err)
		return
	}
	if err = service.ValidateMonitorAIModel(auth.Owner, input.Model, input.Group); err != nil {
		monitorAIError(c, err)
		return
	}
	if err = service.ValidateMonitorAISources(input.Sources, auth.Execution); err != nil {
		monitorAIError(c, err)
		return
	}
	finish, err := service.BeginMonitorAIGeneration(c.Request.Context(), auth.Owner.Id, c.Param("id"))
	if err != nil {
		monitorAIError(c, err)
		return
	}
	defer finish()
	system, prompt, err := service.MonitorAIPrompt(input.Sources)
	if err != nil {
		monitorAIError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(common.WithMonitorAIContext(c.Request.Context()), 110*time.Second)
	defer cancel()
	// Cancellation/revocation can happen in a different request while the model
	// is working. Revalidation stops the outbound request as well as its result.
	jobID := c.Param("id")
	done := make(chan struct{})
	stopped := make(chan struct{})
	defer func() { close(done); <-stopped }()
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				current, err := service.AuthorizeMonitorAI(device, jobID, input.LeaseToken)
				if err != nil || current.FlowRevision != auth.FlowRevision || current.ConfigVersion != auth.ConfigVersion || service.ValidateMonitorAIModel(current.Owner, input.Model, input.Group) != nil {
					cancel()
					return
				}
			}
		}
	}()
	content, err := runMonitorAIRelay(ctx, auth.Owner, input.Model, input.Group, system, prompt)
	current, authErr := service.AuthorizeMonitorAI(device, c.Param("id"), input.LeaseToken)
	if authErr != nil || current.FlowRevision != auth.FlowRevision || current.ConfigVersion != auth.ConfigVersion {
		monitorAIError(c, service.ErrMonitorAIAuthorization)
		return
	}
	if err != nil || ctx.Err() != nil {
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Monitor AI generation failed; review usage before trying again"))
		return
	}
	if err = service.ValidateMonitorAIModel(current.Owner, input.Model, input.Group); err != nil {
		monitorAIError(c, err)
		return
	}
	proposal, err := service.ValidateMonitorAIProposal([]byte(content), input.Sources, auth.Execution)
	if err != nil {
		monitorAIError(c, err)
		return
	}
	common.ApiSuccess(c, proposal)
}

// monitorAIResponseCapture is deliberately bounded and never writes to disk.
// Gin supplies its own responseWriter around this isolated HTTP writer.
type monitorAIResponseCapture struct {
	header http.Header
	body   bytes.Buffer
	status int
	failed bool
}

func (capture *monitorAIResponseCapture) Header() http.Header { return capture.header }
func (capture *monitorAIResponseCapture) Flush()              {}
func (capture *monitorAIResponseCapture) WriteHeader(status int) {
	if capture.status == 0 {
		capture.status = status
	}
}
func (capture *monitorAIResponseCapture) Write(data []byte) (int, error) {
	if capture.body.Len()+len(data) > 2<<20 {
		capture.failed = true
		return 0, errors.New("monitor AI response exceeds limit")
	}
	if capture.status == 0 {
		capture.status = http.StatusOK
	}
	return capture.body.Write(data)
}

func runMonitorAIRelay(ctx context.Context, owner *model.User, name, group, system, prompt string) (string, error) {
	body, err := common.Marshal(gin.H{
		"model": name, "group": group, "stream": false, "max_tokens": 8192,
		"response_format": gin.H{"type": "json_object"},
		"messages":        []gin.H{{"role": "system", "content": system}, {"role": "user", "content": prompt}},
	})
	if err != nil {
		return "", err
	}
	capture := &monitorAIResponseCapture{header: make(http.Header)}
	// A private Gin engine executes only the already-authorized internal chain.
	// No network self-call, synthetic login session or persisted token exists.
	engine := gin.New()
	engine.POST("/pg/chat/completions", func(c *gin.Context) {
		base := owner.ToBaseUser()
		base.WriteContext(c)
		c.Set("id", owner.Id)
		c.Set("username", owner.Username)
		c.Set(common.RequestIdKey, common.NewRequestId())
		common.SetContextKey(c, constant.ContextKeyUsingGroup, owner.Group)
		storage := common.NewMemoryBodyStorage(body)
		c.Set(common.KeyBodyStorage, storage)
		defer common.CleanupBodyStorage(c)
		c.Next()
	}, middleware.Distribute(), func(c *gin.Context) {
		channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		if !service.MonitorAISelectedChannelAllowed(channelID, name) {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		// Admin channel transforms must not replace this constrained system
		// instruction, force a stream, lift limits or capture sample text.
		common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{})
		channelSettings, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
		if ok {
			channelSettings.SystemPrompt = ""
			channelSettings.SystemPromptOverride = false
			common.SetContextKey(c, constant.ContextKeyChannelSetting, channelSettings)
		}
		Playground(c)
	})
	request := &http.Request{Method: http.MethodPost, URL: &url.URL{Path: "/pg/chat/completions"}, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body))}
	request = request.WithContext(ctx)
	engine.ServeHTTP(capture, request)
	if capture.failed || capture.status != http.StatusOK {
		return "", errors.New("monitor AI relay failed")
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if common.DecodeJsonBody(bytes.NewReader(capture.body.Bytes()), &response) != nil || len(response.Choices) != 1 || response.Choices[0].Message.Content == "" || response.Choices[0].FinishReason != "stop" {
		return "", errors.New("monitor AI relay returned an incomplete proposal")
	}
	return response.Choices[0].Message.Content, nil
}
