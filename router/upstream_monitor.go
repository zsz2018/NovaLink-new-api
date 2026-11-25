package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerUpstreamMonitorRoutes(api *gin.RouterGroup) {
	monitor := api.Group("/upstream_monitor", middleware.DisableCache())
	management := monitor.Group("", middleware.RootAuth(), func(c *gin.Context) {
		if _, ok := middleware.GetSessionAuthIdentity(c); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "A dashboard login session is required"})
			return
		}
		c.Next()
	})
	management.GET("/state", controller.GetUpstreamMonitorState)
	management.GET("/browser_setup", controller.GetUpstreamMonitorBrowserSetup)
	management.GET("/profit/detail", controller.GetUpstreamMonitorProfitDetail)
	management.PUT("/config", controller.PutUpstreamMonitorConfig)
	management.POST("/devices/pair", middleware.CriticalRateLimit(), controller.PairUpstreamMonitorDevice)
	management.DELETE("/devices/:id", controller.DeleteUpstreamMonitorDevice)
	management.DELETE("/devices/:id/record", controller.DeleteUpstreamMonitorDeviceRecord)
	management.POST("/jobs", controller.CreateUpstreamMonitorJob)
	management.GET("/jobs/:id", controller.GetUpstreamMonitorJob)
	management.POST("/jobs/:id/control", controller.ControlUpstreamMonitorJob)
	bridge := monitor.Group("/bridge", func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && origin != "null" && !isMonitorExtensionOrigin(origin) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "Browser extension origin required"})
			return
		}
		if origin != "" && origin != "null" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})
	bridge.OPTIONS("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	bridge.POST("/pair", middleware.CriticalRateLimit(), controller.BridgePairUpstreamMonitor)
	bridge.POST("/restart", controller.UpstreamMonitorDeviceAuth, controller.BridgeRestartUpstreamMonitor)
	bridge.POST("/poll", controller.UpstreamMonitorDeviceAuth, controller.BridgePollUpstreamMonitor)
	bridge.POST("/jobs/:id/events", controller.UpstreamMonitorDeviceAuth, controller.BridgeEventUpstreamMonitor)
	bridge.POST("/jobs/:id/flow", controller.UpstreamMonitorDeviceAuth, controller.BridgeSaveUpstreamMonitorFlow)
	bridge.POST("/jobs/:id/result", controller.UpstreamMonitorDeviceAuth, controller.BridgeResultUpstreamMonitor)
	bridge.POST("/jobs/:id/ai/models", controller.UpstreamMonitorDeviceAuth, controller.BridgeMonitorAIModels)
	bridge.POST("/jobs/:id/ai/generate", controller.UpstreamMonitorDeviceAuth, controller.BridgeGenerateMonitorAI)
}

func isMonitorExtensionOrigin(origin string) bool {
	const prefix = "chrome-extension://"
	if len(origin) != len(prefix)+32 || origin[:len(prefix)] != prefix {
		return false
	}
	for _, r := range origin[len(prefix):] {
		if r < 'a' || r > 'p' {
			return false
		}
	}
	return true
}
