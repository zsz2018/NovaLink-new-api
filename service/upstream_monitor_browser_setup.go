package service

import (
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	monitorBrowserSetupTTL             = 25
	monitorBrowserSetupMaxDevices      = 512
	monitorBrowserSetupMaxOrigins      = 512
	monitorBrowserSetupMaxOriginLength = 512
)

// MonitorBrowserSetup is a short-lived UI hint reported by an authenticated
// browser bridge. It is intentionally kept outside the database.
type MonitorBrowserSetup struct {
	SidebarOpen    bool     `json:"sidebar_open"`
	GrantedOrigins []string `json:"granted_origins"`
}

type monitorBrowserSetupEntry struct {
	setup  MonitorBrowserSetup
	seenAt int64
}

var monitorBrowserSetupStore = struct {
	sync.Mutex
	entries map[string]monitorBrowserSetupEntry
}{entries: make(map[string]monitorBrowserSetupEntry)}

func RecordMonitorBrowserSetup(deviceID string, setup *MonitorBrowserSetup) {
	if strings.TrimSpace(deviceID) == "" {
		return
	}
	now := time.Now().Unix()
	monitorBrowserSetupStore.Lock()
	defer monitorBrowserSetupStore.Unlock()
	cleanupMonitorBrowserSetupLocked(now)
	if setup == nil {
		delete(monitorBrowserSetupStore.entries, deviceID)
		return
	}
	sanitized := MonitorBrowserSetup{
		SidebarOpen:    setup.SidebarOpen,
		GrantedOrigins: sanitizeMonitorBrowserOrigins(setup.GrantedOrigins),
	}
	if _, exists := monitorBrowserSetupStore.entries[deviceID]; !exists && len(monitorBrowserSetupStore.entries) >= monitorBrowserSetupMaxDevices {
		var oldestID string
		var oldestSeen int64
		for id, entry := range monitorBrowserSetupStore.entries {
			if oldestID == "" || entry.seenAt < oldestSeen {
				oldestID, oldestSeen = id, entry.seenAt
			}
		}
		if oldestID != "" {
			delete(monitorBrowserSetupStore.entries, oldestID)
		}
	}
	monitorBrowserSetupStore.entries[deviceID] = monitorBrowserSetupEntry{setup: sanitized, seenAt: now}
}

func MonitorBrowserSetupState() map[string]MonitorBrowserSetup {
	now := time.Now().Unix()
	monitorBrowserSetupStore.Lock()
	defer monitorBrowserSetupStore.Unlock()
	cleanupMonitorBrowserSetupLocked(now)
	state := make(map[string]MonitorBrowserSetup, len(monitorBrowserSetupStore.entries))
	for deviceID, entry := range monitorBrowserSetupStore.entries {
		state[deviceID] = MonitorBrowserSetup{
			SidebarOpen:    entry.setup.SidebarOpen,
			GrantedOrigins: append([]string{}, entry.setup.GrantedOrigins...),
		}
	}
	return state
}

func cleanupMonitorBrowserSetupLocked(now int64) {
	for deviceID, entry := range monitorBrowserSetupStore.entries {
		if entry.seenAt+monitorBrowserSetupTTL < now {
			delete(monitorBrowserSetupStore.entries, deviceID)
		}
	}
}

func sanitizeMonitorBrowserOrigins(origins []string) []string {
	result := make([]string, 0, min(len(origins), monitorBrowserSetupMaxOrigins))
	seen := make(map[string]struct{}, min(len(origins), monitorBrowserSetupMaxOrigins))
	for _, raw := range origins[:min(len(origins), monitorBrowserSetupMaxOrigins)] {
		origin := strings.TrimSpace(raw)
		if !validMonitorBrowserOriginPattern(origin) || len(result) >= monitorBrowserSetupMaxOrigins {
			continue
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		result = append(result, origin)
	}
	return result
}

func validMonitorBrowserOriginPattern(pattern string) bool {
	if len(pattern) == 0 || len(pattern) > monitorBrowserSetupMaxOriginLength || strings.ContainsAny(pattern, "\t\r\n\\?#") {
		return false
	}
	if pattern == "<all_urls>" {
		return true
	}
	scheme, remainder, found := strings.Cut(pattern, "://")
	if !found || (scheme != "http" && scheme != "https" && scheme != "*") {
		return false
	}
	host, path, found := strings.Cut(remainder, "/")
	if !found || host == "" || path != "*" || strings.ContainsAny(host, "@ ") {
		return false
	}
	if host == "*" {
		return true
	}
	if strings.HasPrefix(host, "*.") {
		host = host[2:]
		if host == "" {
			return false
		}
	}
	if strings.Contains(host, "*") {
		return false
	}
	parsed, err := url.Parse("https://" + host + "/")
	return err == nil && parsed.Host == host && parsed.Hostname() != ""
}
