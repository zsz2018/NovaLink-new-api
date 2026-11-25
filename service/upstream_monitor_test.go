package service

import (
	"context"
	"encoding/base64"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMonitorBrowserSetupFreshReportsRemainAdvisoryAndExpire(t *testing.T) {
	monitorBrowserSetupStore.Lock()
	previous := monitorBrowserSetupStore.entries
	monitorBrowserSetupStore.entries = make(map[string]monitorBrowserSetupEntry)
	monitorBrowserSetupStore.Unlock()
	t.Cleanup(func() {
		monitorBrowserSetupStore.Lock()
		monitorBrowserSetupStore.entries = previous
		monitorBrowserSetupStore.Unlock()
	})

	assert.Empty(t, MonitorBrowserSetupState(), "older devices have unknown setup state")
	setup := &MonitorBrowserSetup{SidebarOpen: true, GrantedOrigins: []string{"https://upstream.example/*"}}
	RecordMonitorBrowserSetup("paired-browser", setup)
	setup.GrantedOrigins[0] = "https://mutated.example/*"
	state := MonitorBrowserSetupState()
	require.Contains(t, state, "paired-browser")
	assert.Equal(t, MonitorBrowserSetup{SidebarOpen: true, GrantedOrigins: []string{"https://upstream.example/*"}}, state["paired-browser"])
	state["paired-browser"].GrantedOrigins[0] = "https://view-mutated.example/*"
	assert.Equal(t, []string{"https://upstream.example/*"}, MonitorBrowserSetupState()["paired-browser"].GrantedOrigins)

	RecordMonitorBrowserSetup("paired-browser", &MonitorBrowserSetup{})
	state = MonitorBrowserSetupState()
	require.Contains(t, state, "paired-browser", "an explicit closed panel differs from an unknown device")
	assert.False(t, state["paired-browser"].SidebarOpen)
	assert.Equal(t, []string{}, state["paired-browser"].GrantedOrigins)
	RecordMonitorBrowserSetup("paired-browser", nil)
	assert.NotContains(t, MonitorBrowserSetupState(), "paired-browser", "an omitted report invalidates earlier readiness")

	RecordMonitorBrowserSetup("stale-browser", &MonitorBrowserSetup{SidebarOpen: true})
	monitorBrowserSetupStore.Lock()
	entry := monitorBrowserSetupStore.entries["stale-browser"]
	entry.seenAt = time.Now().Unix() - 60
	monitorBrowserSetupStore.entries["stale-browser"] = entry
	monitorBrowserSetupStore.Unlock()
	assert.NotContains(t, MonitorBrowserSetupState(), "stale-browser")
}

func TestMonitorBrowserSetupBoundsDevicesAndSanitizesOriginPatterns(t *testing.T) {
	monitorBrowserSetupStore.Lock()
	previous := monitorBrowserSetupStore.entries
	monitorBrowserSetupStore.entries = make(map[string]monitorBrowserSetupEntry)
	monitorBrowserSetupStore.Unlock()
	t.Cleanup(func() {
		monitorBrowserSetupStore.Lock()
		monitorBrowserSetupStore.entries = previous
		monitorBrowserSetupStore.Unlock()
	})

	patterns := []string{
		"https://upstream.example/*", "https://upstream.example/*", "*://*.example.org/*", "http://127.0.0.1:3001/*", "https://*/*", "<all_urls>",
		"chrome-extension://example/*", "file:///*", "https://user:secret@upstream.example/*", "https://upstream.example/account", "https://upstream.example/*?token=secret", "https://bad*host.example/*", "https://upstream.example\\path/*", "https://" + strings.Repeat("x", 513) + "/*",
	}
	RecordMonitorBrowserSetup("patterns", &MonitorBrowserSetup{GrantedOrigins: patterns})
	assert.Equal(t, []string{"https://upstream.example/*", "*://*.example.org/*", "http://127.0.0.1:3001/*", "https://*/*", "<all_urls>"}, MonitorBrowserSetupState()["patterns"].GrantedOrigins)

	manyOrigins := make([]string, 513)
	for i := range manyOrigins {
		manyOrigins[i] = "https://site-" + strconv.Itoa(i) + ".example/*"
	}
	RecordMonitorBrowserSetup("patterns", &MonitorBrowserSetup{GrantedOrigins: manyOrigins})
	assert.Len(t, MonitorBrowserSetupState()["patterns"].GrantedOrigins, 512)
	for i := range 513 {
		RecordMonitorBrowserSetup("browser-"+strconv.Itoa(i), &MonitorBrowserSetup{})
	}
	assert.Len(t, MonitorBrowserSetupState(), 512)
	RecordMonitorBrowserSetup("", &MonitorBrowserSetup{})
	assert.Len(t, MonitorBrowserSetupState(), 512)
}

func monitorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previous, previousType := model.DB, common.MainDatabaseType()
	dialect := os.Getenv("MONITOR_TEST_DIALECT")
	var driver gorm.Dialector
	switch dialect {
	case "mysql":
		driver = mysql.Open(os.Getenv("MONITOR_TEST_DSN"))
	case "postgres":
		driver = postgres.Open(os.Getenv("MONITOR_TEST_DSN"))
	default:
		dialect = "sqlite"
		driver = sqlite.Open(":memory:")
	}
	db, err := gorm.Open(driver, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	versionQuery := "SELECT version()"
	if dialect == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	var databaseVersion string
	require.NoError(t, sqlDB.QueryRow(versionQuery).Scan(&databaseVersion))
	t.Logf("Database dialect=%s version=%s", dialect, databaseVersion)
	common.SetMainDatabaseType(common.DatabaseType(dialect))
	model.DB = db
	previousLogDB, previousLogType, previousMaster := model.LOG_DB, common.LogDatabaseType(), common.IsMasterNode
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode = false
	require.NoError(t, model.InitLogDB())
	common.IsMasterNode = previousMaster
	t.Setenv("UPSTREAM_MONITOR_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("m", 32))))
	t.Cleanup(func() {
		model.DB = previous
		common.SetMainDatabaseType(previousType)
		common.IsMasterNode = false
		_ = model.InitLogDB()
		model.LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogType)
		common.IsMasterNode = previousMaster
		_ = sqlDB.Close()
	})
	tables := []any{&model.MonitorConfiguration{}, &model.MonitorDevice{}, &model.MonitorJob{}, &model.MonitorSnapshot{}, &model.User{}, &model.Channel{}, &model.Ability{}, &model.Option{}, &model.Model{}, &model.SystemTask{}, &model.SystemTaskLock{}}
	require.NoError(t, db.AutoMigrate(tables...))
	require.NoError(t, db.AutoMigrate(tables...))
	for _, table := range tables {
		require.NoError(t, db.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table).Error)
	}
	return db
}

func monitorTestConfig() MonitorConfig {
	config := monitorDefaultConfig()
	config.Sites = []MonitorSite{{ID: "site", Name: "Upstream", Origin: "https://upstream.example", Currency: "CNY", FXRate: 1}}
	config.Accounts = []MonitorAccount{{ID: "account", SiteID: "site", Name: "Account", Username: "user", ExternalID: "42", Password: "upstream-secret", FlowID: "flow", PurchaseFactor: 1, FeeBearer: "other", Enabled: true}}
	config.Flows = []MonitorFlow{{ID: "flow", SiteID: "site", Name: "Capture", Revision: 1, CaptureSeconds: 10, Steps: []MonitorStep{{Action: "navigate", URL: "https://upstream.example/dashboard"}}, Extraction: &MonitorExtraction{
		Version: 1,
		Requests: []MonitorRequestRule{
			{ID: "self", URL: "https://upstream.example/self", Method: "GET", PageURLs: []string{"https://upstream.example/dashboard"}},
			{ID: "wallet", URL: "https://upstream.example/wallet", Method: "GET", PageURLs: []string{"https://upstream.example/dashboard"}},
			{ID: "groups", URL: "https://upstream.example/groups", Method: "GET", PageURLs: []string{"https://upstream.example/dashboard"}},
			{ID: "prices", URL: "https://upstream.example/prices", Method: "GET", PageURLs: []string{"https://upstream.example/dashboard"}},
		},
		Mappings: []MonitorMapping{
			{ID: "identity", RequestID: "self", Target: "account_id", Iterate: "single", Fields: map[string]MonitorFieldMapping{"value": {Kind: "path", Path: "/data/id"}}},
			{ID: "balance", RequestID: "wallet", Target: "balances", Iterate: "single", Fields: map[string]MonitorFieldMapping{"amount": {Kind: "path", Path: "/data/quota", Divide: &MonitorFieldMapping{Kind: "path", Path: "/data/unit"}}, "currency": {Kind: "constant", Value: common.RawMessage(`"CNY"`)}, "kind": {Kind: "constant", Value: common.RawMessage(`"wallet"`)}}},
			{ID: "groups", RequestID: "groups", Target: "groups", Root: "/data", Iterate: "entries", Fields: map[string]MonitorFieldMapping{"name": {Kind: "key"}, "ratio": {Kind: "path"}}},
			{ID: "models", RequestID: "prices", Target: "models", Root: "/data", Iterate: "array", Fields: map[string]MonitorFieldMapping{
				"name": {Kind: "path", Path: "/name"}, "group": {Kind: "constant", Value: common.RawMessage(`"standard"`)}, "currency": {Kind: "constant", Value: common.RawMessage(`"CNY"`)}, "billing_unit": {Kind: "constant", Value: common.RawMessage(`"token"`)}, "token_unit": {Kind: "constant", Value: common.RawMessage(`1000000`)}, "input": {Kind: "path", Path: "/input"}, "output": {Kind: "path", Path: "/output"}, "includes_group": {Kind: "constant", Value: common.RawMessage(`false`)},
			}},
		},
	}}}
	config.Scenario.FeeBearer = "other"
	return config
}

func monitorTestDevice(t *testing.T) *model.MonitorDevice {
	t.Helper()
	owner := model.User{Username: "monitor-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&owner).Error)
	pair, err := CreateMonitorPair(owner.Id, "Browser")
	require.NoError(t, err)
	credentials, err := CompleteMonitorPair(pair["code"].(string), "Browser")
	require.NoError(t, err)
	_, err = CompleteMonitorPair(pair["code"].(string), "Replay")
	require.Error(t, err)
	device, err := AuthenticateMonitorDevice(credentials["token"])
	require.NoError(t, err)
	return device
}

func monitorTestBatch(execution MonitorExecution) MonitorBatchResult {
	amount, ratio, tokenUnit, input, output, included := 60.0, 0.5, 1000000.0, 2.0, 6.0, false
	result := MonitorBatchResult{SchemaVersion: 1, FlowRevision: execution.Flow.Revision, Complete: true, Missing: []string{}, Data: MonitorData{
		AccountID: "42", Balances: []MonitorBalance{{Amount: &amount, Currency: "CNY", Kind: "wallet"}}, Groups: []MonitorGroup{{Name: "standard", Ratio: &ratio}}, Models: []MonitorModelPrice{{Name: "monitor-priced", Group: "standard", Currency: "CNY", BillingUnit: "token", TokenUnit: &tokenUnit, Input: &input, Output: &output, IncludesGroup: &included}}, Topups: []MonitorTopup{}, Warnings: []string{},
	}}
	for _, request := range execution.Flow.Extraction.Requests {
		result.Sources = append(result.Sources, MonitorResultSource{RequestID: request.ID, URL: request.URL, Method: request.Method, PageURL: "https://upstream.example/dashboard", CapturedAt: common.GetTimestamp()})
	}
	return result
}

func monitorTestOfficialConfig(t *testing.T) MonitorConfig {
	t.Helper()
	catalog, err := GetMonitorOfficialCatalog("")
	require.NoError(t, err)
	config := monitorTestConfig()
	config.Sites[0].Currency, config.Sites[0].FXRate = "USD", 7
	config.Flows[0].Extraction.Mappings[1].Fields["currency"] = MonitorFieldMapping{Kind: "constant", Value: common.RawMessage(`"USD"`)}
	config.Flows[0].Extraction.Mappings[3].Fields["currency"] = MonitorFieldMapping{Kind: "constant", Value: common.RawMessage(`"USD"`)}
	config.Accounts[0].PurchaseFactor, config.Accounts[0].FeeBearer, config.Accounts[0].FeeRate = 0.8, "self", 0.02
	config.Accounts[0].OfficialEstimate = &MonitorOfficialEstimatePolicy{CatalogVersion: catalog.Version, Models: []MonitorOfficialEstimateModel{
		{Name: "monitor-priced", Group: "standard", EntryID: "openai:gpt-4.1"},
		{Name: "official-alias", Group: "standard", EntryID: "openai:gpt-4.1"},
	}}
	config.Scenario.InputTokens, config.Scenario.OutputTokens, config.Scenario.CacheRead, config.Scenario.CacheWrite, config.Scenario.Requests = 100000, 1000, 2000, 1000, 2
	return config
}

func TestMonitorOfficialPolicyStrictAccountProtocol(t *testing.T) {
	config := monitorTestOfficialConfig(t)
	encoded, err := common.Marshal(config)
	require.NoError(t, err)
	var decoded MonitorConfig
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	assert.Equal(t, config, decoded)
	require.NoError(t, validateMonitorConfig(decoded))
	for _, policy := range []string{
		`null`, `{"catalog_version":null,"models":[]}`, `{"models":null}`, `{"models":[null]}`,
		`{"models":[{"name":null}]}`, `{"models":[{"group":null}]}`, `{"models":[{"entry_id":null}]}`,
		`{"auto":true}`, `{"models":[{"price":1}]}`, `{"models":{}}`, `{"catalog_version":1}`,
		`{"source_url":"https://forged.example"}`, `{"entries":[]}`,
	} {
		body := `{"official_estimate":` + policy + `}`
		var account MonitorAccount
		require.Error(t, common.DecodeJsonBodyStrict(strings.NewReader(body), &account), body)
		require.Error(t, common.UnmarshalJsonStr(body, &account), "persisted policies are equally strict")
	}
	for _, body := range []string{`{"official_estimate":null}`, `{"official_estimate":{"catalog_version":"old","models":[]}}`} {
		var extraction MonitorExtraction
		require.Error(t, common.UnmarshalJsonStr(body, &extraction), "old extraction contracts are rejected without migration")
		var proposal MonitorAIProposal
		require.Error(t, common.UnmarshalJsonStr(`{"extraction":`+body+`}`, &proposal), "AI drafts cannot introduce account comparison settings")
	}
	for _, body := range []string{`{"official_catalog":{}}`, `{"official_catalog_latest_version":"old"}`} {
		var execution MonitorExecution
		require.Error(t, common.DecodeJsonBodyStrict(strings.NewReader(body), &execution))
	}
	for _, tc := range []struct {
		name string
		edit func(*MonitorConfig)
	}{
		{"non USD", func(c *MonitorConfig) { c.Sites[0].Currency = "CNY" }},
		{"empty models", func(c *MonitorConfig) { c.Accounts[0].OfficialEstimate.Models = nil }},
		{"too many models", func(c *MonitorConfig) {
			c.Accounts[0].OfficialEstimate.Models = make([]MonitorOfficialEstimateModel, 501)
		}},
		{"duplicate identity", func(c *MonitorConfig) { p := c.Accounts[0].OfficialEstimate; p.Models = append(p.Models, p.Models[0]) }},
		{"missing version", func(c *MonitorConfig) { c.Accounts[0].OfficialEstimate.CatalogVersion = "" }},
		{"unknown version", func(c *MonitorConfig) { c.Accounts[0].OfficialEstimate.CatalogVersion = "unknown" }},
		{"unknown entry", func(c *MonitorConfig) { c.Accounts[0].OfficialEstimate.Models[0].EntryID = "openai:unknown" }},
		{"empty name", func(c *MonitorConfig) { c.Accounts[0].OfficialEstimate.Models[0].Name = "" }},
		{"long name", func(c *MonitorConfig) { c.Accounts[0].OfficialEstimate.Models[0].Name = strings.Repeat("n", 256) }},
		{"long group", func(c *MonitorConfig) { c.Accounts[0].OfficialEstimate.Models[0].Group = strings.Repeat("g", 129) }},
		{"control name", func(c *MonitorConfig) { c.Accounts[0].OfficialEstimate.Models[0].Name = "name\n" }},
		{"control group", func(c *MonitorConfig) { c.Accounts[0].OfficialEstimate.Models[0].Group = "group\x00" }},
		{"invalid UTF8", func(c *MonitorConfig) { c.Accounts[0].OfficialEstimate.Models[0].Name = string([]byte{0xff}) }},
		{"non exact identity", func(c *MonitorConfig) { c.Accounts[0].OfficialEstimate.Models[0].Name = " alias " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := monitorTestOfficialConfig(t)
			tc.edit(&config)
			require.Error(t, validateMonitorConfig(config))
		})
	}
	selection := config.Accounts[0].OfficialEstimate.Models[0]
	config.Accounts[0].OfficialEstimate.Models = nil
	for i := range 500 {
		selection.Name = "alias-" + strconv.Itoa(i)
		config.Accounts[0].OfficialEstimate.Models = append(config.Accounts[0].OfficialEstimate.Models, selection)
	}
	config.Accounts[0].OfficialEstimate.Models[0].Name = strings.Repeat("n", 255)
	config.Accounts[0].OfficialEstimate.Models[0].Group = strings.Repeat("g", 128)
	require.NoError(t, validateMonitorConfig(config), "selection can be saved before a group snapshot exists")
}

func TestMonitorCapturedPricingAvailabilityIsIndependentOfCoreCompleteness(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		price        *MonitorModelPrice
	}{
		{"absent", "unavailable", nil},
		{"empty token row", "unavailable", &MonitorModelPrice{BillingUnit: "token"}},
		{"empty metadata row", "unavailable", &MonitorModelPrice{TokenUnit: new(1000000.0), IncludesGroup: new(false)}},
		{"partial zero price", "partial", &MonitorModelPrice{BillingUnit: "token", Input: new(0.0)}},
		{"unknown group inclusion", "partial", &MonitorModelPrice{BillingUnit: "token", Input: new(2.0), Output: new(6.0), TokenUnit: new(1000000.0)}},
		{"missing output", "partial", &MonitorModelPrice{BillingUnit: "token", Input: new(2.0), IncludesGroup: new(false), TokenUnit: new(1000000.0)}},
		{"pending rules", "partial", &MonitorModelPrice{BillingUnit: "token", PricingPending: true}},
		{"unsupported", "partial", &MonitorModelPrice{BillingUnit: "unsupported", Notes: "USD per second"}},
		{"unknown request price", "partial", &MonitorModelPrice{BillingUnit: "request"}},
		{"unknown expression", "partial", &MonitorModelPrice{BillingUnit: "expression"}},
		{"complete free quote", "available", &MonitorModelPrice{BillingUnit: "token", TokenUnit: new(1000000.0), Input: new(0.0), Output: new(0.0), IncludesGroup: new(false)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := monitorTestOfficialConfig(t)
			execution := MonitorExecution{Site: config.Sites[0], Account: config.Accounts[0], Flow: config.Flows[0]}
			execution.Flow.Extraction.Mappings = execution.Flow.Extraction.Mappings[:3]
			execution.Flow.Extraction.Requests = execution.Flow.Extraction.Requests[:3]
			require.NoError(t, validateMonitorExtraction(execution.Flow.Extraction, execution.Site))
			batch := monitorTestBatch(execution)
			batch.Data.Balances[0].Currency = "USD"
			batch.Data.PricingStatus = "available"
			batch.Data.Models = []MonitorModelPrice{}
			if tc.price != nil {
				price := *tc.price
				price.Name, price.Group, price.Currency = "monitor-priced", "standard", "USD"
				batch.Data.Models = append(batch.Data.Models, price)
			}
			data, err := validateMonitorBatch(batch, execution, common.GetTimestamp())
			require.NoError(t, err)
			assert.Equal(t, tc.status, data.PricingStatus, "caller status cannot promote captured facts")
			assert.Equal(t, batch.Data.Models, data.Models)
		})
	}
	config := monitorTestOfficialConfig(t)
	for _, tc := range []struct {
		name string
		edit func(*MonitorBatchResult)
	}{
		{"missing account", func(b *MonitorBatchResult) { b.Data.AccountID = "" }},
		{"different account", func(b *MonitorBatchResult) { b.Data.AccountID = "other" }},
		{"missing balance", func(b *MonitorBatchResult) { b.Data.Balances = nil }},
		{"unknown balance", func(b *MonitorBatchResult) { b.Data.Balances[0].Amount = nil }},
		{"missing groups", func(b *MonitorBatchResult) { b.Data.Groups = nil }},
		{"unknown ratio", func(b *MonitorBatchResult) { b.Data.Groups[0].Ratio = nil }},
		{"missing selected price response", func(b *MonitorBatchResult) { b.Sources = b.Sources[:3] }},
		{"pricing error", func(b *MonitorBatchResult) { b.Missing = []string{"models.pricing"} }},
		{"wrong source page", func(b *MonitorBatchResult) { b.Sources[3].PageURL = "https://upstream.example/other" }},
		{"forged status", func(b *MonitorBatchResult) { b.Data.PricingStatus = "estimated" }},
		{"unsafe group note", func(b *MonitorBatchResult) { b.Data.Groups[0].CostUnsupported = "unsafe\ntext" }},
		{"oversized group note", func(b *MonitorBatchResult) { b.Data.Groups[0].CostUnsupported = strings.Repeat("x", 513) }},
		{"injected derived price", func(b *MonitorBatchResult) {
			b.Data.Models = []MonitorModelPrice{{PriceSource: &MonitorPriceSource{Kind: "official_estimate"}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			execution := MonitorExecution{Site: config.Sites[0], Account: config.Accounts[0], Flow: config.Flows[0]}
			batch := monitorTestBatch(execution)
			batch.Data.Balances[0].Currency, batch.Data.Models = "USD", nil
			tc.edit(&batch)
			_, err := validateMonitorBatch(batch, execution, common.GetTimestamp())
			require.Error(t, err, "comparison settings never repair failed captures")
		})
	}
	for _, source := range []string{`null`, `{}`, `{"kind":"official_estimate"}`, `{"kind":"captured"}`} {
		var price MonitorModelPrice
		require.Error(t, common.UnmarshalJsonStr(`{"price_source":`+source+`}`, &price))
	}
}

func TestMonitorOfficialComparisonPreservesCapturedPricePrecedence(t *testing.T) {
	config := monitorTestOfficialConfig(t)
	account, site := config.Accounts[0], config.Sites[0]
	for _, tc := range []struct {
		name      string
		price     *MonitorModelPrice
		estimated bool
	}{
		{"absent", nil, true},
		{"empty", &MonitorModelPrice{}, true},
		{"metadata only", &MonitorModelPrice{BillingUnit: "token", TokenUnit: new(1000000.0), IncludesGroup: new(false)}, true},
		{"zero input", &MonitorModelPrice{BillingUnit: "token", Input: new(0.0)}, false},
		{"partial output", &MonitorModelPrice{BillingUnit: "token", Output: new(6.0)}, false},
		{"partial cache", &MonitorModelPrice{BillingUnit: "token", CacheRead: new(0.0)}, false},
		{"pending", &MonitorModelPrice{BillingUnit: "token", PricingPending: true}, false},
		{"notes only", &MonitorModelPrice{Notes: "Unresolved pricing rule"}, false},
		{"unsupported", &MonitorModelPrice{BillingUnit: "unsupported"}, false},
		{"unknown unit", &MonitorModelPrice{BillingUnit: "credits"}, false},
		{"request unit", &MonitorModelPrice{BillingUnit: "request"}, false},
		{"expression unit", &MonitorModelPrice{BillingUnit: "expression"}, false},
		{"bad expression", &MonitorModelPrice{BillingUnit: "expression", BillingExpression: "bad("}, false},
		{"tier data", &MonitorModelPrice{BillingUnit: "token", Tiers: []MonitorPriceTier{{InputThreshold: new(int64(10))}}}, false},
		{"bad tier rules", &MonitorModelPrice{BillingUnit: "token", TierMode: "invalid"}, false},
		{"free captured quote", &MonitorModelPrice{BillingUnit: "token", TokenUnit: new(1000000.0), Input: new(0.0), Output: new(0.0), IncludesGroup: new(false)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := MonitorData{Groups: []MonitorGroup{{Name: "standard", Ratio: new(0.5)}}}
			if tc.price != nil {
				price := *tc.price
				price.Name, price.Group, price.Currency = "official-alias", "standard", "USD"
				data.Models = []MonitorModelPrice{price}
			}
			before, err := common.Marshal(data)
			require.NoError(t, err)
			quote, reason := monitorComparisonPrice(data, account, site, "official-alias", "standard")
			require.NotNil(t, quote, reason)
			assert.Equal(t, tc.estimated, quote.PriceSource != nil)
			if !tc.estimated {
				assert.Equal(t, data.Models[0], *quote)
			}
			after, err := common.Marshal(data)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
	for _, tc := range []struct {
		name, reason string
		edit         func(*MonitorData, *MonitorAccount, *MonitorSite)
	}{
		{"no policy", "Upstream model price is missing", func(_ *MonitorData, a *MonitorAccount, _ *MonitorSite) { a.OfficialEstimate = nil }},
		{"unselected model", "Upstream model price is missing", func(_ *MonitorData, a *MonitorAccount, _ *MonitorSite) {
			a.OfficialEstimate.Models = a.OfficialEstimate.Models[:1]
		}},
		{"unknown group", "Upstream group ratio is unknown", func(d *MonitorData, _ *MonitorAccount, _ *MonitorSite) { d.Groups[0].Name = "other" }},
		{"unknown ratio", "Upstream group ratio is unknown", func(d *MonitorData, _ *MonitorAccount, _ *MonitorSite) { d.Groups[0].Ratio = nil }},
		{"unsupported group", "Upstream group cost comparison is unsupported", func(d *MonitorData, _ *MonitorAccount, _ *MonitorSite) {
			d.Groups[0].CostUnsupported = "Subscription group has no cash price"
		}},
		{"stale version", "Monitor official catalog is unavailable", func(_ *MonitorData, a *MonitorAccount, _ *MonitorSite) { a.OfficialEstimate.CatalogVersion = "missing" }},
		{"tampered entry", "Invalid monitor official estimate policy", func(_ *MonitorData, a *MonitorAccount, _ *MonitorSite) {
			a.OfficialEstimate.Models[1].EntryID = "forged:entry"
		}},
		{"non USD", "Invalid monitor official estimate policy", func(_ *MonitorData, _ *MonitorAccount, s *MonitorSite) { s.Currency = "CNY" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := monitorTestOfficialConfig(t)
			data := MonitorData{Groups: []MonitorGroup{{Name: "standard", Ratio: new(0.5)}}}
			tc.edit(&data, &config.Accounts[0], &config.Sites[0])
			quote, reason := monitorComparisonPrice(data, config.Accounts[0], config.Sites[0], "official-alias", "standard")
			assert.Nil(t, quote)
			assert.Equal(t, tc.reason, reason)
		})
	}
	data := MonitorData{Groups: []MonitorGroup{{Name: "standard", Ratio: new(0.5)}}, Models: []MonitorModelPrice{{Name: "official-alias", Group: "standard", Currency: "CNY"}}}
	quote, _ := monitorComparisonPrice(data, account, site, "official-alias", "standard")
	require.NotNil(t, quote)
	assert.Nil(t, quote.PriceSource)
	data.Models = nil
	account.OfficialEstimate.Models[1].EntryID = "openai:gpt-6.1-sol"
	quote, reason := monitorComparisonPrice(data, account, site, "official-alias", "standard")
	require.NotNil(t, quote, reason)
	scenario := config.Scenario
	scenario.InputTokens, scenario.CacheRead, scenario.CacheWrite = 270000, 2000, 1
	cost, tier, err := monitorUpstreamPrice(*quote, scenario)
	require.NoError(t, err)
	assert.Equal(t, "long_context", tier)
	assert.InDelta(t, (1.08+0.015+0.0004+0.000005)*2, cost, 1e-12)
}

func TestMonitorOfficialCatalogRetainsPinnedVersionsAndStateCopies(t *testing.T) {
	monitorTestDB(t)
	config := monitorTestOfficialConfig(t)
	catalog, err := GetMonitorOfficialCatalog("")
	require.NoError(t, err)
	encoded, err := common.Marshal(catalog)
	require.NoError(t, err)
	var newer MonitorOfficialCatalog
	require.NoError(t, common.Unmarshal(encoded, &newer))
	newer.Version = "official-next"
	newer.Entries[0].Quote.Input = new(999.0)
	payload, err := common.Marshal(map[string]any{"catalogs": []MonitorOfficialCatalog{*catalog, newer}})
	require.NoError(t, err)
	registry, err := loadMonitorOfficialCatalogRegistry(payload)
	require.NoError(t, err)
	previous := monitorOfficialCatalogs
	monitorOfficialCatalogs = func() (monitorOfficialCatalogRegistry, error) { return registry, nil }
	t.Cleanup(func() { monitorOfficialCatalogs = previous })
	other := config.Accounts[0]
	other.ID = "other-account"
	config.Accounts = append(config.Accounts, other)
	_, err = SaveMonitorConfig(config)
	require.NoError(t, err)
	state, err := MonitorState()
	require.NoError(t, err)
	assert.Equal(t, newer.Version, state["official_catalog_latest_version"])
	catalogs := state["official_catalogs"].([]MonitorOfficialCatalog)
	require.Len(t, catalogs, 2)
	assert.Equal(t, newer.Version, catalogs[0].Version)
	assert.Equal(t, *catalog, catalogs[1])
	catalogs[1].Entries[0].Quote.Input = new(0.0)
	catalogs[1].Entries[0].SourceURL = "https://mutated.example"
	again, err := GetMonitorOfficialCatalog(catalog.Version)
	require.NoError(t, err)
	assert.Equal(t, catalog, again)
	loaded, err := GetMonitorConfig()
	require.NoError(t, err)
	assert.Equal(t, catalog.Version, loaded.Accounts[0].OfficialEstimate.CatalogVersion)
	_, err = GetMonitorOfficialCatalog("missing-version")
	require.Error(t, err)
}

func TestMonitorOfficialComparisonRecomputesExistingSnapshotWithoutBrowserChanges(t *testing.T) {
	for _, adapter := range []string{"", "newapi", "sub2api"} {
		t.Run("adapter="+adapter, func(t *testing.T) {
			db := monitorTestDB(t)
			device := monitorTestDevice(t)
			config := monitorTestOfficialConfig(t)
			policy := config.Accounts[0].OfficialEstimate
			config.Accounts[0].OfficialEstimate = nil
			config.Accounts[0].DeviceID = device.ID
			config.Flows[0].Extraction.Adapter = adapter
			config.Flows[0].Extraction.Requests = config.Flows[0].Extraction.Requests[:3]
			config.Flows[0].Extraction.Mappings = config.Flows[0].Extraction.Mappings[:3]
			if adapter != "" {
				config.Flows[0].Extraction.Mappings = []MonitorMapping{}
			}
			channel := model.Channel{Name: "Official comparison", Key: "test", Status: common.ChannelStatusEnabled, Models: "local-estimate", Group: "default"}
			require.NoError(t, db.Create(&channel).Error)
			require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "local-estimate", ChannelId: channel.Id, Enabled: true}).Error)
			require.NoError(t, db.Create(&[]model.Option{{Key: "billing_setting.billing_mode", Value: `{"local-estimate":"tiered_expr"}`}, {Key: "billing_setting.billing_expr", Value: `{"local-estimate":"tier(\"local\", p * 10 + c * 20 + cr + cc * 12)"}`}}).Error)
			config.Bindings = []MonitorBinding{{ID: "official-binding", ChannelID: channel.Id, AccountID: "account", UpstreamGroup: "standard", LocalModel: "local-estimate", UpstreamModel: "official-alias"}}
			config, err := SaveMonitorConfig(config)
			require.NoError(t, err)
			collect, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
			require.NoError(t, err)
			polled, err := PollMonitorDevice(device, "", "")
			require.NoError(t, err)
			execution := polled["job"].(MonitorExecution)
			batch := monitorTestBatch(execution)
			batch.Data.Balances[0].Currency, batch.Data.Models = "USD", []MonitorModelPrice{}
			_, err = SubmitMonitorBrowserResult(device, collect.ID, MonitorResultRequest{LeaseToken: "wrong", Result: batch})
			require.Error(t, err)
			batch.Data.AccountID = "another"
			_, err = SubmitMonitorBrowserResult(device, collect.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: batch})
			require.Error(t, err)
			batch.Data.AccountID = "42"
			result, err := SubmitMonitorBrowserResult(device, collect.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: batch})
			require.NoError(t, err)
			assert.Equal(t, true, result["complete"])
			assert.Equal(t, true, result["confirmed"])
			assert.NotContains(t, result, "estimated")
			assert.NotContains(t, result, "estimated_models")
			assert.Equal(t, "unavailable", result["data"].(MonitorData).PricingStatus)
			_, err = SubmitMonitorBrowserResult(device, collect.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: batch})
			require.Error(t, err)
			var before model.MonitorSnapshot
			require.NoError(t, db.First(&before, "job_id = ?", collect.ID).Error)
			selection := MonitorProfitSelection{BindingID: "official-binding", LocalGroup: "default", LocalModel: "local-estimate"}
			detail, err := GetMonitorProfitDetail(selection)
			require.NoError(t, err)
			assert.Equal(t, "pending", detail.Row.Status)
			assert.Nil(t, detail.Row.Cost)
			config.Accounts[0].OfficialEstimate = policy
			saved, err := SaveMonitorConfig(config)
			require.NoError(t, err)
			_, err = SaveMonitorConfig(config)
			require.ErrorIs(t, err, ErrMonitorConflict)
			detail, err = GetMonitorProfitDetail(selection)
			require.NoError(t, err)
			require.Equal(t, "ready", detail.Row.Status)
			assert.True(t, detail.Row.Estimated)
			require.NotNil(t, detail.Row.PriceSource)
			assert.Equal(t, policy.CatalogVersion, detail.Row.PriceSource.CatalogVersion)
			assert.Equal(t, "openai:gpt-4.1", detail.Row.PriceSource.EntryID)
			require.NotNil(t, detail.Upstream)
			assert.True(t, detail.Upstream.Estimated)
			assert.Equal(t, detail.Row.PriceSource, detail.Upstream.PriceSource)
			assert.False(t, detail.Local.Estimated)
			require.NotNil(t, detail.Row.Cost)
			assert.InDelta(t, 1.205232, *detail.Row.Cost, 1e-12, "official base * captured0.5 ratio once * FX7 * purchase0.8 * fee1.02")
			assert.Equal(t, detail.Row.Cost, detail.Upstream.Amount)
			state, err := MonitorState()
			require.NoError(t, err)
			rows := state["profit_rows"].([]MonitorProfitRow)
			require.Len(t, rows, 1)
			assert.True(t, rows[0].Estimated)
			assert.Equal(t, detail.Row.PriceSource, rows[0].PriceSource)
			var after model.MonitorSnapshot
			require.NoError(t, db.First(&after, "id = ?", before.ID).Error)
			assert.Equal(t, before, after)
			assert.NotContains(t, string(after.Data), "price_source")
			assert.NotContains(t, string(after.Data), "official")
			loaded, err := GetMonitorConfig()
			require.NoError(t, err)
			loaded.Accounts[0].OfficialEstimate.Models[1].Name = "upstream-secret"
			_, err = SaveMonitorConfig(loaded)
			require.EqualError(t, err, "Recorded passwords must use a password variable")
			record, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", FlowID: "flow", Mode: "record"})
			require.NoError(t, err)
			polled, err = PollMonitorDevice(device, "", "")
			require.NoError(t, err)
			execution = polled["job"].(MonitorExecution)
			assert.Nil(t, execution.Account.OfficialEstimate)
			encoded, err := common.Marshal(execution)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "official_")
			var frozen model.MonitorJob
			require.NoError(t, db.First(&frozen, "id = ?", record.ID).Error)
			frozenJSON, err := monitorDecrypt(string(frozen.Frozen))
			require.NoError(t, err)
			assert.NotContains(t, frozenJSON, "official_")
			execution.Flow.Name = "Browser actions remain independent"
			_, err = SaveMonitorBrowserFlow(device, record.ID, MonitorFlowSaveRequest{LeaseToken: execution.LeaseToken, Flow: execution.Flow})
			require.NoError(t, err)
			loaded, err = GetMonitorConfig()
			require.NoError(t, err)
			assert.Equal(t, saved.Accounts[0].OfficialEstimate, loaded.Accounts[0].OfficialEstimate)
			_, err = ControlMonitorJob(record.ID, "cancel")
			require.NoError(t, err)
			collect, err = CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
			require.NoError(t, err)
			polled, err = PollMonitorDevice(device, "", "")
			require.NoError(t, err)
			execution = polled["job"].(MonitorExecution)
			assert.Nil(t, execution.Account.OfficialEstimate)
			batch.FlowRevision = execution.Flow.Revision
			forged := batch
			forged.Data.Models = []MonitorModelPrice{{PriceSource: detail.Row.PriceSource}}
			_, err = SubmitMonitorBrowserResult(device, collect.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: forged})
			require.EqualError(t, err, "Monitor snapshots must contain captured prices only")
			batch.Complete = false
			result, err = SubmitMonitorBrowserResult(device, collect.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: batch})
			require.NoError(t, err)
			assert.Equal(t, false, result["complete"])
			assert.Equal(t, false, result["confirmed"])
			loaded.Accounts[0].OfficialEstimate = nil
			_, err = SaveMonitorConfig(loaded)
			require.NoError(t, err)
			detail, err = GetMonitorProfitDetail(selection)
			require.NoError(t, err)
			assert.Equal(t, "pending", detail.Row.Status)
			assert.False(t, detail.Row.Estimated)
			assert.Nil(t, detail.Row.Cost)
			require.NoError(t, db.First(&after, "id = ?", before.ID).Error)
			assert.Equal(t, before, after)
		})
	}
}

func monitorTestBillingRules() *MonitorBillingMapping {
	return &MonitorBillingMapping{Rules: []MonitorBillingRule{
		{Match: "all", Conditions: []MonitorBillingCondition{{Path: "/input", Op: "gte", Value: new(0.0)}, {Path: "/output", Op: "gte", Value: new(0.0)}}, Unit: "token"},
		{Match: "any", Conditions: []MonitorBillingCondition{{Path: "/request_price", Op: "gt", Value: new(0.0)}}, Unit: "request", RequestPrice: &MonitorBillingPrice{Kind: "path", Path: "/request_price", Scale: new(0.01)}},
		{Match: "all", Conditions: []MonitorBillingCondition{{Path: "/seconds_price", Op: "exists"}}, Unit: "unsupported", Notes: "USD per second"},
	}}
}

func TestMonitorBillingRulesStrictProtocolRoundTrip(t *testing.T) {
	config := monitorTestConfig()
	mapping := &config.Flows[0].Extraction.Mappings[3]
	delete(mapping.Fields, "billing_unit")
	mapping.Billing = monitorTestBillingRules()
	request := MonitorFlowSaveRequest{LeaseToken: "test-lease", Flow: config.Flows[0]}
	encoded, err := common.Marshal(request)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"value":0`)
	var decoded MonitorFlowSaveRequest
	require.NoError(t, common.DecodeJsonBodyStrict(strings.NewReader(string(encoded)), &decoded))
	assert.Equal(t, request, decoded)
	require.NoError(t, validateMonitorExtraction(decoded.Flow.Extraction, config.Sites[0]))
	require.NoError(t, ValidateMonitorExtractionDraft(decoded.Flow.Extraction, config.Sites[0]))
	var persisted MonitorFlowSaveRequest
	require.NoError(t, common.Unmarshal(encoded, &persisted))
	assert.Equal(t, request, persisted)

	for _, tc := range []struct {
		name    string
		billing string
	}{
		{"null billing", `null`},
		{"null rules", `{"rules":null}`},
		{"null rule", `{"rules":[null]}`},
		{"null match", `{"rules":[{"match":null}]}`},
		{"null conditions", `{"rules":[{"conditions":null}]}`},
		{"null condition", `{"rules":[{"conditions":[null]}]}`},
		{"null path", `{"rules":[{"conditions":[{"path":null}]}]}`},
		{"null op", `{"rules":[{"conditions":[{"op":null}]}]}`},
		{"null threshold", `{"rules":[{"conditions":[{"value":null}]}]}`},
		{"null unit", `{"rules":[{"unit":null}]}`},
		{"null price", `{"rules":[{"request_price":null}]}`},
		{"null price kind", `{"rules":[{"request_price":{"kind":null}}]}`},
		{"null price path", `{"rules":[{"request_price":{"path":null}}]}`},
		{"null scale", `{"rules":[{"request_price":{"scale":null}}]}`},
		{"null notes", `{"rules":[{"notes":null}]}`},
		{"unknown billing field", `{"rules":[],"auto":true}`},
		{"unknown rule field", `{"rules":[{"model":"specific-model"}]}`},
		{"condition source", `{"rules":[{"conditions":[{"source":"prices"}]}]}`},
		{"condition kind", `{"rules":[{"conditions":[{"kind":"parent"}]}]}`},
		{"price source", `{"rules":[{"request_price":{"source":"prices"}}]}`},
		{"price constant", `{"rules":[{"request_price":{"value":0.01}}]}`},
		{"price arithmetic", `{"rules":[{"request_price":{"multiply":{"kind":"path","path":"/ratio"}}}]}`},
		{"string threshold", `{"rules":[{"conditions":[{"value":"0"}]}]}`},
		{"boolean threshold", `{"rules":[{"conditions":[{"value":false}]}]}`},
		{"overflow threshold", `{"rules":[{"conditions":[{"value":1e400}]}]}`},
		{"string scale", `{"rules":[{"request_price":{"scale":"1"}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"flow":{"extraction":{"mappings":[{"billing":` + tc.billing + `}]}}}`
			var request MonitorFlowSaveRequest
			require.Error(t, common.DecodeJsonBodyStrict(strings.NewReader(body), &request))
			require.Error(t, common.UnmarshalJsonStr(body, &request), "persisted rules use the same strict boundary")
		})
	}
}

func TestMonitorBillingRulesValidateDraftAndPublishedMappings(t *testing.T) {
	for _, tc := range []struct {
		name  string
		valid bool
		edit  func(*MonitorMapping)
	}{
		{"all numeric operators and missing", true, func(m *MonitorMapping) {
			m.Billing.Rules[0].Conditions = []MonitorBillingCondition{
				{Path: "/input", Op: "gt", Value: new(0.0)}, {Path: "/input", Op: "gte", Value: new(1e15)},
				{Path: "/input", Op: "eq", Value: new(0.0)}, {Path: "/input", Op: "lt", Value: new(1.0)},
				{Path: "/input", Op: "lte", Value: new(1.0)}, {Path: "/request_price", Op: "missing"},
			}
		}},
		{"escaped pointers and optional scale", true, func(m *MonitorMapping) {
			m.Billing.Rules[0].Conditions[0].Path = "/price~1input/~0value"
			m.Billing.Rules[1].RequestPrice.Scale = nil
			m.Billing.Rules[0].Notes = " "
		}},
		{"maximum rules conditions and notes", true, func(m *MonitorMapping) {
			m.Billing.Rules[0].Notes = strings.Repeat("界", 170) + "ab"
			for len(m.Billing.Rules[0].Conditions) < 8 {
				m.Billing.Rules[0].Conditions = append(m.Billing.Rules[0].Conditions, MonitorBillingCondition{Path: "/optional", Op: "missing"})
			}
			for len(m.Billing.Rules) < 8 {
				m.Billing.Rules = append(m.Billing.Rules, m.Billing.Rules[0])
			}
		}},
		{"nested models with tiers", true, func(m *MonitorMapping) {
			m.Iterate, m.ChildRoot = "nested_array", "/models"
			m.Fields["group"] = MonitorFieldMapping{Kind: "parent", Path: "/group_name"}
			m.Tiers = &MonitorTierMapping{Path: "/tiers", Mode: "graduated", Basis: "input", Fields: map[string]MonitorFieldMapping{"input_threshold": {Kind: "path", Path: "/threshold"}, "input": {Kind: "path", Path: "/input"}}}
		}},
		{"empty rules", false, func(m *MonitorMapping) { m.Billing.Rules = nil }},
		{"too many rules", false, func(m *MonitorMapping) { m.Billing.Rules = make([]MonitorBillingRule, 9) }},
		{"empty conditions", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions = nil }},
		{"too many conditions", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions = make([]MonitorBillingCondition, 9) }},
		{"unknown match", false, func(m *MonitorMapping) { m.Billing.Rules[0].Match = "auto" }},
		{"unknown operator", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Op = "contains" }},
		{"missing threshold", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Value = nil }},
		{"negative threshold", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Value = new(-1.0) }},
		{"large threshold", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Value = new(1e15 + 1) }},
		{"NaN threshold", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Value = new(math.NaN()) }},
		{"infinite threshold", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Value = new(math.Inf(1)) }},
		{"exists threshold", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Op = "exists" }},
		{"missing threshold forbidden", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Op = "missing" }},
		{"empty condition pointer", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Path = "" }},
		{"non-pointer condition", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Path = "../price" }},
		{"malformed condition pointer", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Path = "/price~2" }},
		{"long condition pointer", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Path = "/" + strings.Repeat("p", 512) }},
		{"deep condition pointer", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Path = strings.Repeat("/p", 41) }},
		{"prototype condition pointer", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Path = "/__proto__/price" }},
		{"credential condition pointer", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Path = "/api_key" }},
		{"control condition pointer", false, func(m *MonitorMapping) { m.Billing.Rules[0].Conditions[0].Path = "/price\n" }},
		{"missing request price", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice = nil }},
		{"constant request price", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Kind = "constant" }},
		{"parent request price", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Kind = "parent" }},
		{"empty price pointer", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Path = "" }},
		{"malformed price pointer", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Path = "/price~" }},
		{"credential price pointer", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Path = "/authorization" }},
		{"deep price pointer", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Path = strings.Repeat("/p", 41) }},
		{"long price pointer", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Path = "/" + strings.Repeat("p", 512) }},
		{"zero scale", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Scale = new(0.0) }},
		{"negative scale", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Scale = new(-1.0) }},
		{"NaN scale", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Scale = new(math.NaN()) }},
		{"infinite scale", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Scale = new(math.Inf(1)) }},
		{"large scale", false, func(m *MonitorMapping) { m.Billing.Rules[1].RequestPrice.Scale = new(1e15 + 1) }},
		{"token request price", false, func(m *MonitorMapping) { m.Billing.Rules[1].Unit = "token" }},
		{"unsupported request price", false, func(m *MonitorMapping) {
			m.Billing.Rules[1].Unit, m.Billing.Rules[1].Notes = "unsupported", "USD per image"
		}},
		{"expression unit", false, func(m *MonitorMapping) { m.Billing.Rules[0].Unit = "expression" }},
		{"unknown unit", false, func(m *MonitorMapping) { m.Billing.Rules[0].Unit = "auto" }},
		{"unsupported missing notes", false, func(m *MonitorMapping) { m.Billing.Rules[2].Notes = "" }},
		{"unsupported blank notes", false, func(m *MonitorMapping) { m.Billing.Rules[2].Notes = "  " }},
		{"notes byte limit", false, func(m *MonitorMapping) { m.Billing.Rules[2].Notes = strings.Repeat("界", 171) }},
		{"notes control", false, func(m *MonitorMapping) { m.Billing.Rules[2].Notes = "USD\nper second" }},
		{"notes redaction", false, func(m *MonitorMapping) { m.Billing.Rules[2].Notes = "[REDACTED] per second" }},
		{"notes invalid UTF8", false, func(m *MonitorMapping) { m.Billing.Rules[2].Notes = string([]byte{0xff}) }},
		{"billing unit field conflict", false, func(m *MonitorMapping) {
			m.Fields["billing_unit"] = MonitorFieldMapping{Kind: "constant", Value: common.RawMessage(`"token"`)}
		}},
		{"request price field conflict", false, func(m *MonitorMapping) {
			m.Fields["request_price"] = MonitorFieldMapping{Kind: "path", Path: "/request_price"}
		}},
		{"expression field conflict", false, func(m *MonitorMapping) {
			m.Fields["billing_expression"] = MonitorFieldMapping{Kind: "path", Path: "/expression"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := monitorTestConfig()
			mapping := &config.Flows[0].Extraction.Mappings[3]
			delete(mapping.Fields, "billing_unit")
			mapping.Billing = monitorTestBillingRules()
			tc.edit(mapping)
			if tc.valid {
				require.NoError(t, validateMonitorConfig(config))
				require.NoError(t, ValidateMonitorExtractionDraft(config.Flows[0].Extraction, config.Sites[0]))
			} else {
				require.EqualError(t, validateMonitorConfig(config), "Invalid monitor billing rules")
				require.EqualError(t, ValidateMonitorExtractionDraft(config.Flows[0].Extraction, config.Sites[0]), "Invalid monitor billing rules")
			}
		})
	}
	for _, index := range []int{0, 1, 2} {
		config := monitorTestConfig()
		config.Flows[0].Extraction.Mappings[index].Billing = monitorTestBillingRules()
		require.EqualError(t, validateMonitorConfig(config), "Invalid monitor billing rules", "billing only belongs to models")
		require.EqualError(t, ValidateMonitorExtractionDraft(config.Flows[0].Extraction, config.Sites[0]), "Invalid monitor billing rules")
	}
}

func TestMonitorProtocolRejectsRemovedResponseAndParserFields(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		request any
	}{
		{"raw event responses", `{"captures":[{"body":"discard"}]}`, &MonitorJobEvent{}},
		{"parser configuration", `{"parser":{"api_key":"fixture-only"}}`, &MonitorConfig{}},
		{"identity heuristic", `{"flow":{"identity_path":"data.id"}}`, &MonitorFlowSaveRequest{}},
		{"request substring filter", `{"flow":{"capture_patterns":["wallet"]}}`, &MonitorFlowSaveRequest{}},
		{"source facts", `{"result":{"data":{"source_facts":[{"value":"discard"}]}}}`, &MonitorResultRequest{}},
		{"response evidence", `{"result":{"data":{"balances":[{"evidence":{"amount":{"body":"discard"}}}]}}}`, &MonitorResultRequest{}},
		{"removed login preparation", `{"flow":{"login":{"before_submit":[]}}}`, &MonitorFlowSaveRequest{}},
		{"response wait body", `{"flow":{"steps":[{"action":"wait_response","request_id":"prices","body":"discard"}]}}`, &MonitorFlowSaveRequest{}},
		{"nested response body", `{"flow":{"steps":[{"action":"condition","steps":[{"action":"checkpoint","body":"discard"}]}]}}`, &MonitorFlowSaveRequest{}},
		{"unknown condition property", `{"flow":{"login":{"success":{"script":"discard"}}}}`, &MonitorFlowSaveRequest{}},
		{"translation object", `{"flow":{"extraction":{"mappings":[{"fields":{"currency":{"kind":"path","translate":{"人民币":{"output":"CNY"}}}}}]}}}`, &MonitorFlowSaveRequest{}},
		{"model coverage", `{"flow":{"extraction":{"model_coverage":"all_selected"}}}`, &MonitorFlowSaveRequest{}},
		{"key join", `{"flow":{"extraction":{"mappings":[{"key_path":"/name"}]}}}`, &MonitorFlowSaveRequest{}},
		{"expand", `{"flow":{"extraction":{"mappings":[{"expand":null}]}}}`, &MonitorFlowSaveRequest{}},
		{"where", `{"flow":{"extraction":{"mappings":[{"where":null}]}}}`, &MonitorFlowSaveRequest{}},
		{"filters", `{"flow":{"extraction":{"mappings":[{"filters":[]}]}}}`, &MonitorFlowSaveRequest{}},
		{"billing overrides", `{"flow":{"extraction":{"mappings":[{"billing_overrides":[]}]}}}`, &MonitorFlowSaveRequest{}},
		{"field values", `{"flow":{"extraction":{"mappings":[{"fields":{"billing_unit":{"kind":"path","values":{}}}}]}}}`, &MonitorFlowSaveRequest{}},
		{"operand values", `{"flow":{"extraction":{"mappings":[{"fields":{"input":{"kind":"path","multiply":{"kind":"path","values":{}}}}}]}}}`, &MonitorFlowSaveRequest{}},
		{"tier values", `{"flow":{"extraction":{"mappings":[{"tiers":{"fields":{"input":{"kind":"path","values":{}}}}}]}}}`, &MonitorFlowSaveRequest{}},
		{"execution billing hints", `{"billing_hints":[]}`, &MonitorExecution{}},
		{"unknown adapter", `{"flow":{"extraction":{"adapter":"other"}}}`, &MonitorFlowSaveRequest{}},
		{"empty adapter", `{"flow":{"extraction":{"adapter":""}}}`, &MonitorFlowSaveRequest{}},
		{"null adapter", `{"flow":{"extraction":{"adapter":null}}}`, &MonitorFlowSaveRequest{}},
		{"object adapter", `{"flow":{"extraction":{"adapter":{}}}}`, &MonitorFlowSaveRequest{}},
		{"numeric adapter", `{"flow":{"extraction":{"adapter":1}}}`, &MonitorFlowSaveRequest{}},
		{"boolean adapter", `{"flow":{"extraction":{"adapter":false}}}`, &MonitorFlowSaveRequest{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, common.DecodeJsonBodyStrict(strings.NewReader(tc.body), tc.request))
		})
	}
	config := monitorTestConfig()
	encoded, err := common.Marshal(config)
	require.NoError(t, err)
	var decoded MonitorConfig
	require.NoError(t, common.DecodeJsonBodyStrict(strings.NewReader(string(encoded)), &decoded))
	assert.Equal(t, config, decoded)
	config.Flows[0].Extraction = nil
	require.EqualError(t, validateMonitorConfig(config), "Invalid monitor extraction rules")
}

func TestMonitorAdapterSaveAndExecution(t *testing.T) {
	for _, adapter := range []string{"newapi", "sub2api"} {
		t.Run(adapter, func(t *testing.T) {
			db := monitorTestDB(t)
			device := monitorTestDevice(t)
			config := monitorTestConfig()
			config.Accounts[0].DeviceID = device.ID
			config.Flows[0].Extraction.Adapter = adapter
			config.Flows[0].Extraction.Mappings = []MonitorMapping{}
			_, err := SaveMonitorConfig(config)
			require.NoError(t, err)
			job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", FlowID: "flow", Mode: "record"})
			require.NoError(t, err)
			polled, err := PollMonitorDevice(device, "", "")
			require.NoError(t, err)
			execution := polled["job"].(MonitorExecution)
			preview := monitorTestBatch(execution)
			request := MonitorFlowSaveRequest{LeaseToken: execution.LeaseToken, Flow: execution.Flow, Preview: &preview}
			request.Flow.Extraction.Requests[0].PageURLs = nil
			_, err = SaveMonitorBrowserFlow(device, job.ID, request)
			require.EqualError(t, err, "Associate each selected request with its source page before saving")
			request.Flow.Extraction.Requests[0].PageURLs = []string{"https://upstream.example/dashboard"}
			request.Preview = nil
			request.Flow.Extraction.Requests[0].Query = map[string]string{"view": "account"}
			_, err = SaveMonitorBrowserFlow(device, job.ID, request)
			require.EqualError(t, err, "Preview the complete structured result before saving mappings")
			request.Preview = &preview
			saved, err := SaveMonitorBrowserFlow(device, job.ID, request)
			require.NoError(t, err)
			loaded, err := GetMonitorConfig()
			require.NoError(t, err)
			assert.Equal(t, saved, loaded.Flows[0])
			assert.Equal(t, adapter, loaded.Flows[0].Extraction.Adapter)
			assert.NotNil(t, loaded.Flows[0].Extraction.Mappings)
			assert.Empty(t, loaded.Flows[0].Extraction.Mappings)
			_, err = ControlMonitorJob(job.ID, "cancel")
			require.NoError(t, err)
			collect, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
			require.NoError(t, err)
			polled, err = PollMonitorDevice(device, "", "")
			require.NoError(t, err)
			execution = polled["job"].(MonitorExecution)
			assert.Equal(t, saved.Extraction, execution.Flow.Extraction)
			missingSource := monitorTestBatch(execution)
			missingSource.Sources = missingSource.Sources[1:]
			_, err = SubmitMonitorBrowserResult(device, collect.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: missingSource})
			require.Error(t, err, "adapters require every selected capture")
			encoded, err := common.Marshal(execution)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "billing_hints")
			assert.NotContains(t, string(encoded), "model_coverage")
			batch := monitorTestBatch(execution)
			result, err := SubmitMonitorBrowserResult(device, collect.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: batch})
			require.NoError(t, err)
			assert.Equal(t, true, result["complete"])
			assert.Equal(t, true, result["confirmed"])
			var snapshot model.MonitorSnapshot
			require.NoError(t, db.First(&snapshot, "job_id = ?", collect.ID).Error)
			var data MonitorData
			require.NoError(t, common.UnmarshalJsonStr(string(snapshot.Data), &data))
			assert.Equal(t, batch.Data.Models, data.Models)
		})
	}
}

func TestMonitorExtractionDraftCanStartEmpty(t *testing.T) {
	for _, adapter := range []string{"", "newapi", "sub2api"} {
		extraction := &MonitorExtraction{Version: 1, Adapter: adapter, Requests: []MonitorRequestRule{}, Mappings: []MonitorMapping{}}
		site := monitorTestConfig().Sites[0]
		require.NoError(t, ValidateMonitorExtractionDraft(extraction, site))
		require.Error(t, validateMonitorExtraction(extraction, site))
	}
}

func TestMonitorRebuildObsoleteFlowInPlace(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Accounts[0].DeviceID = device.ID
	config.Flows[0].Login = monitorTestLogin()
	config, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	config.Flows[0].Extraction.Mappings[3].Fields["billing_unit"] = MonitorFieldMapping{Kind: "auto"}
	encoded, err := common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.MonitorConfiguration{}).Where("id = ?", 1).Update("content", model.LongText(encoded)).Error)
	_, err = CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.Error(t, err, "obsolete rules never execute")
	job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", FlowID: "flow", Mode: "record"})
	require.NoError(t, err, "recording admits an explicit rebuild of the same saved flow")
	polled, err := PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	execution := polled["job"].(MonitorExecution)
	assert.Equal(t, "flow", execution.Flow.ID)
	assert.Equal(t, config.Flows[0].Revision, execution.Flow.Revision)
	assert.Equal(t, config.Flows[0].Login, execution.Flow.Login)
	assert.Equal(t, config.Flows[0].Steps, execution.Flow.Steps)
	execution.Flow.Extraction = monitorTestConfig().Flows[0].Extraction
	preview := monitorTestBatch(execution)
	saved, err := SaveMonitorBrowserFlow(device, job.ID, MonitorFlowSaveRequest{LeaseToken: execution.LeaseToken, Flow: execution.Flow, Preview: &preview})
	require.NoError(t, err)
	assert.Equal(t, "flow", saved.ID)
	assert.Equal(t, config.Flows[0].Revision+1, saved.Revision)
	assert.Equal(t, config.Flows[0].Login, saved.Login)
	assert.Equal(t, config.Flows[0].Steps, saved.Steps)
	loaded, err := GetMonitorConfig()
	require.NoError(t, err)
	require.Len(t, loaded.Flows, 1)
	assert.Equal(t, saved, loaded.Flows[0])
	_, err = ControlMonitorJob(job.ID, "cancel")
	require.NoError(t, err)
	_, err = CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.NoError(t, err)
}

func TestMonitorSaveNewFlowPreservesUnrelatedObsoleteFlow(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Accounts[0].DeviceID = device.ID
	config.Sites = append(config.Sites, MonitorSite{ID: "recording-site", Name: "Recording", Origin: "https://recording.example", Currency: "CNY", FXRate: 1})
	config.Accounts = append(config.Accounts, MonitorAccount{ID: "recording-account", SiteID: "recording-site", Name: "Recording account", DeviceID: device.ID, Password: "recording-secret", PurchaseFactor: 1, FeeBearer: "other", Enabled: true})
	config.Bindings = []MonitorBinding{{ID: "binding", AccountID: "account", ChannelID: 1, UpstreamGroup: "standard"}}
	config, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	config.Flows[0].Extraction.Mappings[3].Fields["billing_unit"] = MonitorFieldMapping{Kind: "auto"}
	encoded, err := common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.MonitorConfiguration{}).Where("id = ?", 1).Update("content", model.LongText(encoded)).Error)
	var before model.MonitorConfiguration
	require.NoError(t, db.First(&before, 1).Error)

	job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "recording-account", Mode: "record"})
	require.NoError(t, err)
	polled, err := PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	execution := polled["job"].(MonitorExecution)
	require.Zero(t, execution.Flow.Revision)
	require.NotEqual(t, config.Flows[0].ID, execution.Flow.ID)
	execution.Flow.Extraction = &MonitorExtraction{Version: 1, Adapter: "newapi", Requests: []MonitorRequestRule{{ID: "prices", URL: "https://recording.example/api/pricing", Method: "GET", PageURLs: []string{"https://recording.example/pricing"}}}, Mappings: []MonitorMapping{}}
	preview := monitorTestBatch(execution)
	preview.Sources[0].PageURL = "https://recording.example/pricing"
	request := MonitorFlowSaveRequest{LeaseToken: execution.LeaseToken, Flow: execution.Flow, Preview: &preview}
	var beforeJob model.MonitorJob
	require.NoError(t, db.First(&beforeJob, "id = ?", job.ID).Error)

	for _, tc := range []struct {
		name   string
		mutate func(*MonitorConfig)
	}{
		{"duplicate flow", func(c *MonitorConfig) { c.Flows = append(c.Flows, c.Flows[0]) }},
		{"flow without site", func(c *MonitorConfig) { c.Flows[0].SiteID = "missing" }},
		{"unpublished saved flow", func(c *MonitorConfig) { c.Flows[0].Revision = 0 }},
		{"flow capture limit", func(c *MonitorConfig) { c.Flows[0].CaptureSeconds = 61 }},
		{"flow step limit", func(c *MonitorConfig) { c.Flows[0].Steps[0].TimeoutMS = 120001 }},
		{"flow count limit", func(c *MonitorConfig) { c.Flows = make([]MonitorFlow, 200) }},
		{"account references another site flow", func(c *MonitorConfig) { c.Accounts[1].FlowID = c.Flows[0].ID }},
		{"recording account moved to another site", func(c *MonitorConfig) { c.Accounts[1].SiteID = "site" }},
		{"binding without account", func(c *MonitorConfig) { c.Bindings[0].AccountID = "missing" }},
		{"invalid scenario", func(c *MonitorConfig) { c.Scenario.USDExchange = 0 }},
		{"changed source origin", func(c *MonitorConfig) { c.Sites[1].Origin = "https://changed.example" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var changed MonitorConfig
			require.NoError(t, common.Unmarshal(encoded, &changed))
			tc.mutate(&changed)
			changedJSON, err := common.Marshal(changed)
			require.NoError(t, err)
			require.NoError(t, db.Model(&model.MonitorConfiguration{}).Where("id = ?", 1).Update("content", model.LongText(changedJSON)).Error)
			_, err = SaveMonitorBrowserFlow(device, job.ID, request)
			require.Error(t, err)
			var stored model.MonitorConfiguration
			require.NoError(t, db.First(&stored, 1).Error)
			assert.Equal(t, before.Version, stored.Version)
			assert.Equal(t, string(changedJSON), string(stored.Content))
			assert.Equal(t, before.Secrets, stored.Secrets)
			var storedJob model.MonitorJob
			require.NoError(t, db.First(&storedJob, "id = ?", job.ID).Error)
			assert.Equal(t, beforeJob, storedJob)
		})
	}
	require.NoError(t, db.Model(&model.MonitorConfiguration{}).Where("id = ?", 1).Update("content", model.LongText(encoded)).Error)

	saved, err := SaveMonitorBrowserFlow(device, job.ID, request)
	require.NoError(t, err, "an unrelated obsolete extraction cannot block a new dedicated flow")
	assert.Equal(t, int64(1), saved.Revision)
	loaded, err := GetMonitorConfig()
	require.NoError(t, err)
	expected := config
	expected.Version++
	expected.Accounts[1].ExternalID = preview.Data.AccountID
	expected.Flows = append(expected.Flows, saved)
	assert.Equal(t, expected, loaded, "only the selected flow and recorded account identity change")
	var stored model.MonitorConfiguration
	require.NoError(t, db.First(&stored, 1).Error)
	assert.Equal(t, before.Secrets, stored.Secrets)
	var storedJob model.MonitorJob
	require.NoError(t, db.First(&storedJob, "id = ?", job.ID).Error)
	assert.Equal(t, saved.Revision, storedJob.FlowRevision)
	assert.Equal(t, loaded.Version, storedJob.ConfigVersion)
	assert.Equal(t, beforeJob.LeaseHash, storedJob.LeaseHash)
	assert.Equal(t, beforeJob.Status, storedJob.Status)
	frozen, err := monitorExecution(&storedJob)
	require.NoError(t, err)
	assert.Equal(t, saved, frozen.Flow)
	assert.Equal(t, preview.Data.AccountID, frozen.Account.ExternalID)
	assert.Equal(t, "recording-secret", frozen.Account.Password)
	_, err = SaveMonitorConfig(loaded)
	require.Error(t, err, "the root full configuration save remains strict")
	_, err = CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.Error(t, err, "the untouched obsolete extraction remains unexecutable")
	_, err = CreateMonitorJob(MonitorJobRequest{AccountID: "recording-account", FlowID: saved.ID, Mode: "collect"})
	require.NoError(t, err)
}

func TestMonitorPersistedExtractionRejectsObsoleteFields(t *testing.T) {
	db := monitorTestDB(t)
	config := monitorTestConfig()
	_, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	encoded, err := common.Marshal(config)
	require.NoError(t, err)
	obsolete := strings.Replace(string(encoded), `"version":1,"requests":`, `"version":1,"model_coverage":"all_selected","requests":`, 1)
	require.NotEqual(t, string(encoded), obsolete)
	require.NoError(t, db.Model(&model.MonitorConfiguration{}).Where("id = ?", 1).Update("content", model.LongText(obsolete)).Error)
	_, err = GetMonitorConfig()
	require.Error(t, err)
	var stored model.MonitorConfiguration
	require.NoError(t, db.First(&stored, 1).Error)
	assert.Equal(t, obsolete, string(stored.Content), "a rejected development flow is never silently rewritten")
}

func TestMonitorConfigSecretsCASAndLargePayload(t *testing.T) {
	db := monitorTestDB(t)
	config := monitorTestConfig()
	for range 20 {
		config.Flows[0].Steps = append(config.Flows[0].Steps, MonitorStep{Action: "input", Selector: "#search", Value: strings.Repeat("a", 4000)})
	}
	saved, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	assert.True(t, saved.Accounts[0].HasPassword)
	assert.Empty(t, saved.Accounts[0].Password)
	var row model.MonitorConfiguration
	require.NoError(t, db.First(&row, 1).Error)
	assert.Greater(t, len(row.Content), 65536)
	assert.NotContains(t, string(row.Content), "upstream-secret")
	assert.NotContains(t, string(row.Secrets), "upstream-secret")
	secrets, err := monitorSecrets()
	require.NoError(t, err)
	assert.Equal(t, "upstream-secret", secrets["password:account"])
	_, err = SaveMonitorConfig(config)
	require.ErrorIs(t, err, ErrMonitorConflict)
	saved.Accounts[0].ClearPassword = true
	saved, err = SaveMonitorConfig(saved)
	require.NoError(t, err)
	assert.False(t, saved.Accounts[0].HasPassword)
	t.Setenv("UPSTREAM_MONITOR_KEY", "")
	loaded, err := GetMonitorConfig()
	require.NoError(t, err)
	assert.Equal(t, saved.Version, loaded.Version)
	assert.False(t, MonitorKeyReady())
	loaded.Accounts[0].Password = "no-key"
	_, err = SaveMonitorConfig(loaded)
	require.Error(t, err)
}

func TestMonitorConfigLoginRoundTrip(t *testing.T) {
	for _, loginJSON := range []string{
		`{"url":"https://upstream.example/login","steps":[{"action":"input","selector":"#email","variable":"username"}]}`,
		`{"url":"https://upstream.example/login","timeout_ms":30000,"success":{"url":"https://upstream.example/dashboard"},"steps":[{"action":"focus","selector":"#account"},{"action":"input","selector":"#account","variable":"username"},{"action":"click","selector":"#next"},{"action":"condition","condition":{"selector":"#terms","state":"visible"},"steps":[{"action":"check","selector":"#terms","url":"https://upstream.example/password"}],"otherwise":[{"action":"checkpoint","text":"Complete verification"}]},{"action":"input","selector":"#password","variable":"password","url":"https://upstream.example/password"},{"action":"key","selector":"#password","value":"Enter"}]}`,
	} {
		config := monitorTestConfig()
		require.NoError(t, common.DecodeJsonBodyStrict(strings.NewReader(loginJSON), &config.Flows[0].Login))
		require.NoError(t, validateMonitorConfig(config))
		encoded, err := common.Marshal(config.Flows[0].Login)
		require.NoError(t, err)
		assert.JSONEq(t, loginJSON, string(encoded))
	}
}

func monitorTestLogin() *MonitorLogin {
	return &MonitorLogin{
		URL: "https://upstream.example/login", TimeoutMS: 30000,
		Steps: []MonitorStep{
			{Action: "input", Selector: "#account", Variable: "username"},
			{Action: "input", Selector: "#password", Variable: "password"},
			{Action: "click", Selector: "#sign-in"},
		},
	}
}

func TestMonitorConfigRejectsUnsafeLoginRules(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*MonitorLogin)
	}{
		{"other origin", func(login *MonitorLogin) { login.URL = "https://upstream.example.invalid/login" }},
		{"embedded user information", func(login *MonitorLogin) { login.URL = "https://user@upstream.example/login" }},
		{"query string", func(login *MonitorLogin) { login.URL += "?redirect=/dashboard" }},
		{"fragment", func(login *MonitorLogin) { login.URL += "#signin" }},
		{"literal credential", func(login *MonitorLogin) { login.Steps[1].Value = "literal-example" }},
		{"no credential", func(login *MonitorLogin) { login.Steps = []MonitorStep{{Action: "checkpoint"}} }},
		{"literal login input", func(login *MonitorLogin) {
			login.Steps[0] = MonitorStep{Action: "input", Selector: "#other", Value: "literal"}
		}},
		{"empty login", func(login *MonitorLogin) { login.Steps = nil }},
		{"foreign credential page", func(login *MonitorLogin) { login.Steps[1].URL = "https://outside.example/password" }},
		{"credential page query", func(login *MonitorLogin) { login.Steps[1].URL = login.URL + "?code=secret" }},
		{"credential page fragment", func(login *MonitorLogin) { login.Steps[1].URL = login.URL + "#password" }},
		{"foreign success URL", func(login *MonitorLogin) { login.Success = &MonitorCondition{URL: "https://outside.example/"} }},
		{"login group context", func(login *MonitorLogin) { login.Steps[0].Group = "default" }},
		{"nested login retry", func(login *MonitorLogin) {
			login.Steps = append(login.Steps, MonitorStep{Action: "condition", Condition: &MonitorCondition{Selector: "#login"}, Steps: []MonitorStep{{Action: "repeat", Condition: &MonitorCondition{Selector: "#submit"}, MaxIterations: 2, Steps: []MonitorStep{{Action: "click", Selector: "#submit"}}}}})
		}},

		{"implicit submit selector", func(login *MonitorLogin) { login.Steps[2].Selector = " " }},
		{"overlong wait", func(login *MonitorLogin) { login.TimeoutMS = 120001 }},
		{"submit credential", func(login *MonitorLogin) { login.Steps[2].Variable = "password" }},
		{"too many nested login actions", func(login *MonitorLogin) {
			children := make([]MonitorStep, 98)
			for i := range children {
				children[i] = MonitorStep{Action: "check", Selector: "#terms"}
			}
			login.Steps = append(login.Steps, MonitorStep{Action: "condition", Condition: &MonitorCondition{Selector: "#terms"}, Steps: children})
		}},
		{"nested literal credential", func(login *MonitorLogin) {
			login.Steps = append(login.Steps, MonitorStep{Action: "condition", Condition: &MonitorCondition{Selector: "#terms"}, Otherwise: []MonitorStep{{Action: "input", Selector: "#other", Value: "secret"}}})
		}},
		{"nested invalid timeout", func(login *MonitorLogin) {
			login.Steps = append(login.Steps, MonitorStep{Action: "condition", Condition: &MonitorCondition{Selector: "#terms"}, Steps: []MonitorStep{{Action: "check", Selector: "#terms", TimeoutMS: -1}}})
		}},
		{"URL condition with state", func(login *MonitorLogin) { login.Success = &MonitorCondition{URL: login.URL, State: "visible"} }},
		{"semantic success selector without name", func(login *MonitorLogin) { login.Success = &MonitorCondition{Selector: `semantic={"role":"button"}`} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := monitorTestConfig()
			config.Flows[0].Login = monitorTestLogin()
			tc.edit(config.Flows[0].Login)
			assert.Error(t, validateMonitorConfig(config))
		})
	}
}

func TestMonitorConfigRejectsSavedAndNewPasswordsInLoginMetadata(t *testing.T) {
	monitorTestDB(t)
	config, err := SaveMonitorConfig(monitorTestConfig())
	require.NoError(t, err)
	assert.Nil(t, config.Flows[0].Login, "public pages do not require automatic login")
	config.Flows[0].Login = monitorTestLogin()
	config.Accounts[0].Password = "new / test / password"
	const message = "Recorded passwords must use a password variable"
	config.Flows[0].Login.Steps[0].Selector = "[data-name='upstream%2dsecret']"
	_, err = SaveMonitorConfig(config)
	require.EqualError(t, err, message)
	config.Flows[0].Login = monitorTestLogin()
	config.Flows[0].Login.URL += "/new%20%2f%20test%20%2f%20password"
	_, err = SaveMonitorConfig(config)
	require.EqualError(t, err, message)
	for _, selector := range []string{"[data-id='upstream%2dsecret']", "[data-id='new%20%2f%20test%20%2f%20password']", `semantic={"label":"upstream-secret"}`} {
		config.Flows[0].Login = monitorTestLogin()
		config.Flows[0].Login.Steps = append(config.Flows[0].Login.Steps, MonitorStep{Action: "condition", Condition: &MonitorCondition{Selector: "#terms"}, Steps: []MonitorStep{{Action: "check", Selector: selector}}})
		_, err = SaveMonitorConfig(config)
		require.EqualError(t, err, message)
	}
	config.Flows[0].Login = monitorTestLogin()
	config.Flows[0].Login.Steps = append(config.Flows[0].Login.Steps, MonitorStep{Action: "check", Selector: "#terms"}, MonitorStep{Action: "check", Selector: "#privacy"})
	saved, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	loaded, err := GetMonitorConfig()
	require.NoError(t, err)
	assert.Equal(t, saved.Flows[0].Login, loaded.Flows[0].Login)
}

func TestMonitorGenericWorkflowValidatesRecursiveActionsAndShape(t *testing.T) {
	for _, tc := range []struct {
		name string
		step MonitorStep
	}{
		{"focus", MonitorStep{Action: "focus", Selector: `semantic={"role":"textbox","name":"Search","scope":"main"}`}},
		{"assert", MonitorStep{Action: "assert", Selector: "#ready", State: "visible", Text: "Ready"}},
		{"response", MonitorStep{Action: "wait_response", RequestID: "prices"}},
		{"otherwise only", MonitorStep{Action: "condition", Condition: &MonitorCondition{URL: "https://upstream.example/dashboard"}, Otherwise: []MonitorStep{{Action: "checkpoint"}}}},
		{"repeat", MonitorStep{Action: "repeat", Condition: &MonitorCondition{Selector: "#next", State: "enabled"}, MaxIterations: 100, Steps: []MonitorStep{{Action: "click", Selector: "#next"}}}},
		{"key default enter", MonitorStep{Action: "key", Selector: "#search"}},
		{"xpath", MonitorStep{Action: "click", Selector: "xpath=//button[@id='next']"}},
		{"Unicode metadata boundary", MonitorStep{ID: strings.Repeat("中", 128), Name: strings.Repeat("中", 128), Action: "assert", Selector: "#" + strings.Repeat("中", 2047), Text: strings.Repeat("中", 512)}},
		{"Unicode semantic selector", MonitorStep{Action: "click", Selector: `semantic={"label":"` + strings.Repeat("中", 512) + `"}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := monitorTestConfig()
			config.Flows[0].Steps = []MonitorStep{tc.step}
			require.NoError(t, validateMonitorConfig(config))
		})
	}
	for _, tc := range []struct {
		name string
		step MonitorStep
	}{
		{"foreign nested navigation", MonitorStep{Action: "condition", Condition: &MonitorCondition{Selector: "#ready"}, Otherwise: []MonitorStep{{Action: "navigate", URL: "https://outside.example/"}}}},
		{"ordinary credential", MonitorStep{Action: "input", Selector: "#email", Variable: "username"}},
		{"nested ordinary credential", MonitorStep{Action: "repeat", Condition: &MonitorCondition{Selector: "#ready"}, MaxIterations: 2, Steps: []MonitorStep{{Action: "input", Selector: "#email", Variable: "username"}}}},
		{"nested literal password", MonitorStep{Action: "condition", Condition: &MonitorCondition{Selector: "#ready"}, Steps: []MonitorStep{{Action: "input", Selector: "#password", Value: "literal"}}}},
		{"missing request", MonitorStep{Action: "wait_response", RequestID: "missing"}},
		{"request invokes step", MonitorStep{ID: "self-cycle", Action: "wait_response", RequestID: "self-cycle"}},
		{"request on click", MonitorStep{Action: "click", Selector: "#next", RequestID: "prices"}},
		{"response selector", MonitorStep{Action: "wait_response", RequestID: "prices", Selector: "#next"}},
		{"condition absent", MonitorStep{Action: "condition", Steps: []MonitorStep{{Action: "checkpoint"}}}},
		{"condition empty", MonitorStep{Action: "condition", Condition: &MonitorCondition{Selector: "#ready"}}},
		{"condition iterations", MonitorStep{Action: "condition", Condition: &MonitorCondition{Selector: "#ready"}, Steps: []MonitorStep{{Action: "checkpoint"}}, MaxIterations: 1}},
		{"repeat zero", MonitorStep{Action: "repeat", Condition: &MonitorCondition{Selector: "#next"}, Steps: []MonitorStep{{Action: "checkpoint"}}}},
		{"repeat unbounded", MonitorStep{Action: "repeat", Condition: &MonitorCondition{Selector: "#next"}, MaxIterations: 101, Steps: []MonitorStep{{Action: "checkpoint"}}}},
		{"repeat empty", MonitorStep{Action: "repeat", Condition: &MonitorCondition{Selector: "#next"}, MaxIterations: 1}},
		{"repeat otherwise", MonitorStep{Action: "repeat", Condition: &MonitorCondition{Selector: "#next"}, MaxIterations: 1, Steps: []MonitorStep{{Action: "checkpoint"}}, Otherwise: []MonitorStep{{Action: "checkpoint"}}}},
		{"children on click", MonitorStep{Action: "click", Selector: "#next", Steps: []MonitorStep{{Action: "checkpoint"}}}},
		{"ordinary DOM binding", MonitorStep{Action: "click", Selector: "#next", URL: "https://upstream.example/login"}},
		{"state on click", MonitorStep{Action: "click", Selector: "#next", State: "visible"}},
		{"state without target", MonitorStep{Action: "wait", State: "visible"}},
		{"missing key target", MonitorStep{Action: "key", Value: "Enter"}},
		{"overlong text", MonitorStep{Action: "checkpoint", Text: strings.Repeat("中", 513)}},
		{"overlong ID", MonitorStep{ID: strings.Repeat("中", 129), Action: "checkpoint"}},
		{"overlong selector", MonitorStep{Action: "click", Selector: strings.Repeat("中", 2049)}},
		{"invalid semantic property", MonitorStep{Action: "click", Selector: `semantic={"label":"Next","exact":true}`}},
		{"semantic number", MonitorStep{Action: "click", Selector: `semantic={"label":12}`}},
		{"semantic null", MonitorStep{Action: "click", Selector: `semantic={"label":null}`}},
		{"semantic missing name", MonitorStep{Action: "click", Selector: `semantic={"role":"button"}`}},
		{"semantic long scope", MonitorStep{Action: "click", Selector: `semantic={"label":"Next","scope":"` + strings.Repeat("a", 513) + `"}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := monitorTestConfig()
			config.Flows[0].Steps = []MonitorStep{tc.step}
			require.Error(t, validateMonitorConfig(config))
		})
	}
}

func TestMonitorWorkflowBoundsCombineLoginAndNestedBranches(t *testing.T) {
	config := monitorTestConfig()
	step := MonitorStep{Action: "checkpoint"}
	for range 3 {
		step = MonitorStep{Action: "condition", Condition: &MonitorCondition{Selector: "#next"}, Steps: []MonitorStep{step}}
	}
	config.Flows[0].Steps = []MonitorStep{step}
	require.NoError(t, validateMonitorConfig(config), "depth four is supported")
	config.Flows[0].Steps = []MonitorStep{{Action: "condition", Condition: &MonitorCondition{Selector: "#next"}, Otherwise: []MonitorStep{step}}}
	require.Error(t, validateMonitorConfig(config), "otherwise branches count toward depth")
	config.Flows[0].Login = monitorTestLogin()
	config.Flows[0].Steps = make([]MonitorStep, 497)
	for i := range config.Flows[0].Steps {
		config.Flows[0].Steps[i] = MonitorStep{Action: "checkpoint"}
	}
	require.NoError(t, validateMonitorConfig(config), "500 total nodes are supported")
	config.Flows[0].Steps = append(config.Flows[0].Steps, MonitorStep{Action: "checkpoint"})
	require.EqualError(t, validateMonitorConfig(config), "Too many monitor steps")
	config.Flows[0].Steps = []MonitorStep{{ID: "duplicate", Action: "checkpoint"}}
	config.Flows[0].Login.Steps[0].ID = "duplicate"
	require.Error(t, validateMonitorConfig(config), "IDs remain unique across login and ordinary trees")
	config.Flows[0].Login = monitorTestLogin()
	for range 97 {
		config.Flows[0].Login.Steps = append(config.Flows[0].Login.Steps, MonitorStep{Action: "checkpoint"})
	}
	require.NoError(t, validateMonitorConfig(config), "100 login nodes are supported")
	config.Flows[0].Login.Steps = append(config.Flows[0].Login.Steps, MonitorStep{Action: "checkpoint"})
	require.Error(t, validateMonitorConfig(config), "101 login nodes are rejected")
}

func TestMonitorTranslationContractPreservesScalarValuesAndRejectsInvalidMaps(t *testing.T) {
	config := monitorTestConfig()
	field := MonitorFieldMapping{Kind: "path", Path: "/currency", Translate: map[string]string{"人民币": "CNY", "0": "CNY", "false": "constructor"}}
	config.Flows[0].Extraction.Mappings[1].Fields["currency"] = field
	require.NoError(t, validateMonitorConfig(config))
	encoded, err := common.Marshal(config)
	require.NoError(t, err)
	var decoded MonitorConfig
	require.NoError(t, common.DecodeJsonBodyStrict(strings.NewReader(string(encoded)), &decoded))
	assert.Equal(t, field, decoded.Flows[0].Extraction.Mappings[1].Fields["currency"])
	for _, translation := range []map[string]string{
		{}, {"": "CNY"}, {"currency": ""}, {"__proto__": "CNY"}, {"constructor": "CNY"}, {"prototype": "CNY"},
		{"a\n": "CNY"}, {"a": "C\nNY"}, {"[redacted]": "CNY"}, {"a": "[REDACTED]"},
		{strings.Repeat("中", 27): "CNY"}, {"a": strings.Repeat("中", 43)},
	} {
		field.Translate = translation
		config.Flows[0].Extraction.Mappings[1].Fields["currency"] = field
		require.Error(t, validateMonitorConfig(config))
	}
	field.Translate = make(map[string]string)
	for i := range 31 {
		field.Translate[strconv.Itoa(i)] = "CNY"
	}
	config.Flows[0].Extraction.Mappings[1].Fields["currency"] = field
	require.Error(t, validateMonitorConfig(config))
	delete(field.Translate, "30")
	require.NoError(t, validateMonitorConfig(config), "30 translations are supported")

	execution := MonitorExecution{Flow: monitorTestConfig().Flows[0], Site: config.Sites[0], Account: config.Accounts[0]}
	batch := monitorTestBatch(execution)
	zero := 0.0
	batch.Data.Balances[0].Amount = &zero
	batch.Data.Models[0].Input = &zero
	data, err := validateMonitorBatch(batch, execution, common.GetTimestamp())
	require.NoError(t, err)
	require.NotNil(t, data.Balances[0].Amount)
	assert.Zero(t, *data.Balances[0].Amount)
	require.NotNil(t, data.Models[0].Input)
	assert.Zero(t, *data.Models[0].Input)
	batch.Data.Balances[0].Currency = "USD"
	_, err = validateMonitorBatch(batch, execution, common.GetTimestamp())
	require.Error(t, err, "translated currencies still pass the configured result currency allowlist")
	batch.Data.Balances[0].Currency = "CNY"
	batch.Data.Balances[0].Amount = nil
	_, err = validateMonitorBatch(batch, execution, common.GetTimestamp())
	require.Error(t, err, "null amounts cannot be promoted to an explicit zero")
}

func TestMonitorDeleteRevokedDeviceDetachesSchedulesAndKeepsHistory(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Accounts[0].DeviceID = device.ID
	config.Accounts[0].IntervalMinutes = 30
	_, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	completed, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.NoError(t, err)
	require.NoError(t, db.Model(completed).Update("status", "completed").Error)
	snapshot := model.MonitorSnapshot{ID: "saved-history", JobID: completed.ID, AccountID: "account", Data: model.LongText(`{"balances":[]}`), Confirmed: true, Complete: true}
	require.NoError(t, db.Create(&snapshot).Error)
	pending, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.NoError(t, err)
	require.EqualError(t, DeleteMonitorDeviceRecord(device.ID), "Revoke the browser before deleting its record")
	require.NoError(t, RevokeMonitorDevice(device.ID))
	require.NoError(t, DeleteMonitorDeviceRecord(device.ID))
	var count int64
	require.NoError(t, db.Model(&model.MonitorDevice{}).Where("id = ?", device.ID).Count(&count).Error)
	assert.Zero(t, count)
	loaded, err := GetMonitorConfig()
	require.NoError(t, err)
	assert.Empty(t, loaded.Accounts[0].DeviceID)
	assert.Zero(t, loaded.Accounts[0].IntervalMinutes)
	assert.Equal(t, "flow", loaded.Accounts[0].FlowID)
	assert.Equal(t, config.Sites, loaded.Sites)
	secrets, err := monitorSecrets()
	require.NoError(t, err)
	assert.Equal(t, "upstream-secret", secrets["password:account"])
	var stored model.MonitorJob
	require.NoError(t, db.First(&stored, "id = ?", completed.ID).Error)
	assert.Equal(t, "completed", stored.Status)
	stored = model.MonitorJob{}
	require.NoError(t, db.First(&stored, "id = ?", pending.ID).Error)
	assert.Equal(t, "cancelled", stored.Status)
	var history model.MonitorSnapshot
	require.NoError(t, db.First(&history, "id = ?", snapshot.ID).Error)
	assert.Equal(t, snapshot.Data, history.Data)
	require.NoError(t, DeleteMonitorDeviceRecord(device.ID))
}

func TestMonitorRetryUnsavedFlowStartsNewDraftAndSavedFlowKeepsIdentity(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Accounts[0].DeviceID = device.ID
	_, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	draft, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "record"})
	require.NoError(t, err)
	assert.Zero(t, draft.FlowRevision)
	for _, action := range []string{"finish_recording", "analyze"} {
		_, err := ControlMonitorJob(draft.ID, action)
		require.EqualError(t, err, "Invalid monitor control action")
	}
	require.NoError(t, db.Model(draft).Updates(map[string]any{"status": "failed", "message": "Timed out waiting for navigation"}).Error)
	retried, err := ControlMonitorJob(draft.ID, "retry")
	require.NoError(t, err)
	assert.NotEqual(t, draft.ID, retried.ID)
	assert.NotEqual(t, draft.FlowID, retried.FlowID)
	assert.Zero(t, retried.FlowRevision)
	assert.Equal(t, "record", retried.Mode)
	assert.Equal(t, "queued", retried.Status)
	loaded, err := GetMonitorConfig()
	require.NoError(t, err)
	require.Len(t, loaded.Flows, 1)
	assert.Equal(t, "flow", loaded.Flows[0].ID)
	saved, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "record", FlowID: "flow"})
	require.NoError(t, err)
	require.NoError(t, db.Model(saved).Update("status", "failed").Error)
	retriedSaved, err := ControlMonitorJob(saved.ID, "retry")
	require.NoError(t, err)
	assert.Equal(t, "flow", retriedSaved.FlowID)
	assert.Equal(t, saved.FlowRevision, retriedSaved.FlowRevision)
	var original model.MonitorJob
	require.NoError(t, db.First(&original, "id = ?", draft.ID).Error)
	assert.Equal(t, "failed", original.Status)
}

func TestMonitorSidebarFlowPublishesPreviewIdentityWithTaskAndRevisionChecks(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Flows = []MonitorFlow{}
	config.Accounts[0].FlowID, config.Accounts[0].ExternalID = "", ""
	config.Accounts[0].DeviceID = device.ID
	_, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "record"})
	require.NoError(t, err)
	polled, err := PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	execution := polled["job"].(MonitorExecution)
	flow := execution.Flow
	flow.Name, flow.Extraction = "Browser configured flow", monitorTestConfig().Flows[0].Extraction
	flow.Extraction.Requests = flow.Extraction.Requests[:3]
	groupMapping := &flow.Extraction.Mappings[2]
	groupMapping.Root, groupMapping.Iterate = "/data/items", "array"
	groupMapping.Fields = map[string]MonitorFieldMapping{
		"name":         {Kind: "path", Path: "/group_code", FallbackPath: "/group_name"},
		"display_name": {Kind: "path", Path: "/group_name"},
		"ratio":        {Kind: "path", Path: "/group_ratio"},
	}
	modelMapping := &flow.Extraction.Mappings[3]
	modelMapping.RequestID, modelMapping.Root = groupMapping.RequestID, groupMapping.Root
	modelMapping.Iterate, modelMapping.ChildRoot = "nested_array", "/models"
	modelMapping.Fields["name"] = MonitorFieldMapping{Kind: "path", Path: "/model_id"}
	modelMapping.Fields["group"] = MonitorFieldMapping{Kind: "parent", Path: "/group_code", FallbackPath: "/group_name"}
	modelMapping.Fields["cache_read"] = MonitorFieldMapping{Kind: "path", Path: "/cache_read"}
	modelMapping.Fields["cache_write"] = MonitorFieldMapping{Kind: "path", Path: "/cache_write"}
	modelMapping.Fields["billing_unit"] = MonitorFieldMapping{Kind: "path", Path: "/charge_type", Translate: map[string]string{"1": "token", "2": "request", "秒": "unsupported"}}
	flow.Steps = []MonitorStep{
		{ID: "open-dashboard", Action: "navigate", URL: "https://upstream.example/dashboard"},
		{ID: "pages", Name: "Capture every page", Action: "repeat", Condition: &MonitorCondition{Selector: "#next", State: "enabled"}, MaxIterations: 3, Steps: []MonitorStep{{Action: "click", Selector: "#next"}, {Action: "wait_response", RequestID: "groups"}}},
	}
	modelMapping.Fields["currency"] = MonitorFieldMapping{Kind: "path", Path: "/currency", Translate: map[string]string{"人民币": "CNY"}}
	flow.Login = monitorTestLogin()
	execution.Flow = flow
	preview := monitorTestBatch(execution)
	request := MonitorFlowSaveRequest{LeaseToken: execution.LeaseToken, Flow: flow}
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.Error(t, err, "a flow cannot be published without a complete preview")
	request.Preview = &preview
	unbound := request
	unbound.Flow.Extraction = monitorTestConfig().Flows[0].Extraction
	unbound.Flow.Extraction.Requests[0].PageURLs = nil
	_, err = SaveMonitorBrowserFlow(device, job.ID, unbound)
	require.EqualError(t, err, "Associate each selected request with its source page before saving")
	badLease := request
	badLease.LeaseToken = "wrong"
	_, err = SaveMonitorBrowserFlow(device, job.ID, badLease)
	require.Error(t, err)
	other := *device
	other.ID = "another-device"
	_, err = SaveMonitorBrowserFlow(&other, job.ID, request)
	require.Error(t, err)
	badSite := request
	badSite.Flow.SiteID = "another-site"
	_, err = SaveMonitorBrowserFlow(device, job.ID, badSite)
	require.Error(t, err)
	badPassword := request
	badPassword.Flow.Steps = append([]MonitorStep{}, flow.Steps...)
	badPassword.Flow.Steps[1] = MonitorStep{Action: "input", Selector: "#password", Value: "upstream-secret"}
	_, err = SaveMonitorBrowserFlow(device, job.ID, badPassword)
	require.Error(t, err)
	loaded, err := GetMonitorConfig()
	require.NoError(t, err)
	assert.Empty(t, loaded.Accounts[0].ExternalID)
	assert.Empty(t, loaded.Flows)
	loaded.Scenario.RevenueFactor = 0.9
	loaded.Accounts[0].ExternalID = "different-account"
	loaded, err = SaveMonitorConfig(loaded)
	require.NoError(t, err)
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.EqualError(t, err, "Captured account identity does not match the configured account")
	loaded.Accounts[0].ExternalID = ""
	const rotatedPassword = "rotated-test-password"
	loaded.Accounts[0].Password = rotatedPassword
	_, err = SaveMonitorConfig(loaded)
	require.NoError(t, err)
	unsafeLogin := request
	unsafeLogin.Flow.Login = monitorTestLogin()
	unsafeLogin.Flow.Login.URL = "https://outside.example/login"
	_, err = SaveMonitorBrowserFlow(device, job.ID, unsafeLogin)
	require.EqualError(t, err, "Monitor navigation is outside the configured origins")
	unsafeLogin.Flow.Login = monitorTestLogin()
	unsafeLogin.Flow.Login.Steps = append(unsafeLogin.Flow.Login.Steps, MonitorStep{Action: "check", Selector: "[data-action='upstream%2Dsecret']"})
	_, err = SaveMonitorBrowserFlow(device, job.ID, unsafeLogin)
	require.EqualError(t, err, "Recorded passwords must use a password variable")
	saved, err := SaveMonitorBrowserFlow(device, job.ID, request)
	require.NoError(t, err)
	assert.Equal(t, int64(1), saved.Revision)
	loaded, err = GetMonitorConfig()
	require.NoError(t, err)
	require.Len(t, loaded.Flows, 1)
	assert.Equal(t, "42", loaded.Accounts[0].ExternalID)
	assert.Equal(t, saved, loaded.Flows[0])
	assert.Equal(t, *groupMapping, loaded.Flows[0].Extraction.Mappings[2])
	assert.Equal(t, *modelMapping, loaded.Flows[0].Extraction.Mappings[3])
	var currentJob model.MonitorJob
	require.NoError(t, db.First(&currentJob, "id = ?", job.ID).Error)
	frozen, err := monitorExecution(&currentJob)
	require.NoError(t, err)
	assert.Equal(t, loaded.Flows[0].Extraction, frozen.Flow.Extraction)
	assert.Equal(t, saved.Login, frozen.Flow.Login)
	assert.Equal(t, 0.9, loaded.Scenario.RevenueFactor)
	secrets, err := monitorSecrets()
	require.NoError(t, err)
	assert.Equal(t, rotatedPassword, secrets["password:account"])
	var stored model.MonitorConfiguration
	require.NoError(t, db.First(&stored, 1).Error)
	assert.NotContains(t, string(stored.Content), "source_facts")
	assert.NotContains(t, string(stored.Content), "captured_at", "preview results are not workflow storage")
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.ErrorIs(t, err, ErrMonitorConflict)
	request.Flow = saved
	request.Flow.Steps = nil // Source pages compile into a navigation plan without manually authored steps.
	request.Preview.FlowRevision = saved.Revision
	saved, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.NoError(t, err)
	assert.Equal(t, int64(2), saved.Revision)
	request.Flow = saved
	request.Preview.FlowRevision = saved.Revision
	require.NoError(t, db.Model(&model.MonitorJob{}).Where("id = ?", job.ID).Update("lease_until", common.GetTimestamp()-1).Error)
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.Error(t, err)
	require.NoError(t, db.Model(&model.MonitorJob{}).Where("id = ?", job.ID).Updates(map[string]any{"lease_until": common.GetTimestamp() + 120, "status": "completed"}).Error)
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.Error(t, err)
}

func TestMonitorSidebarFlowEditsRetainVerifiedMappingsWithoutNewPreview(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Accounts[0].DeviceID = device.ID
	_, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", DeviceID: device.ID, FlowID: "flow", Mode: "record"})
	require.NoError(t, err)
	polled, err := PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	execution := polled["job"].(MonitorExecution)
	flow := execution.Flow
	flow.Name = "Updated login and navigation"
	flow.CaptureSeconds = 20
	flow.Steps = []MonitorStep{{Action: "navigate", URL: "https://upstream.example/dashboard"}}
	flow.Login = monitorTestLogin()
	flow.Login.Steps = append(flow.Login.Steps,
		MonitorStep{Action: "check", Selector: "#terms"},
		MonitorStep{Action: "condition", Condition: &MonitorCondition{Selector: "#privacy", State: "hidden"}, Steps: []MonitorStep{{Action: "click", Selector: "#open-privacy"}}},
		MonitorStep{Action: "check", Selector: "#privacy", TimeoutMS: 5000},
	)
	request := MonitorFlowSaveRequest{LeaseToken: execution.LeaseToken, Flow: flow}

	saved, err := SaveMonitorBrowserFlow(device, job.ID, request)
	require.NoError(t, err, "editing page actions must retain the already verified mappings without recapturing data")
	assert.Equal(t, int64(2), saved.Revision)
	assert.Equal(t, execution.Flow.Extraction, saved.Extraction)
	assert.Equal(t, flow.Login, saved.Login)
	loaded, err := GetMonitorConfig()
	require.NoError(t, err)
	assert.Equal(t, "42", loaded.Accounts[0].ExternalID)
	assert.Equal(t, saved, loaded.Flows[0])
	var snapshots int64
	require.NoError(t, db.Model(&model.MonitorSnapshot{}).Count(&snapshots).Error)
	assert.Zero(t, snapshots, "saving configuration must not manufacture a new collection snapshot")
	var storedJob model.MonitorJob
	require.NoError(t, db.First(&storedJob, "id = ?", job.ID).Error)
	frozen, err := monitorExecution(&storedJob)
	require.NoError(t, err)
	assert.Equal(t, saved, frozen.Flow)

	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.ErrorIs(t, err, ErrMonitorConflict, "old revisions must still be rejected")
	request.Flow = saved
	changed := *saved.Extraction
	changed.Requests = append([]MonitorRequestRule(nil), saved.Extraction.Requests...)
	changed.Requests[0].URL = "https://upstream.example/another-self"
	request.Flow.Extraction = &changed
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.EqualError(t, err, "Preview the complete structured result before saving mappings")
	request.Flow = saved
	loaded.Accounts[0].ExternalID = "another-account"
	loaded, err = SaveMonitorConfig(loaded)
	require.NoError(t, err)
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.EqualError(t, err, "Preview the complete structured result before saving mappings")
	loaded.Accounts[0].ExternalID = ""
	loaded, err = SaveMonitorConfig(loaded)
	require.NoError(t, err)
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.EqualError(t, err, "Preview the complete structured result before saving mappings")
	loaded.Accounts[0].ExternalID = "42"
	loaded.Flows[0].Extraction = &changed
	_, err = SaveMonitorConfig(loaded)
	require.NoError(t, err)
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.EqualError(t, err, "Preview the complete structured result before saving mappings", "a concurrent mapping edit cannot reuse the task's previous verification")
}

func TestMonitorBrowserResultCompletesOnceWithoutParserOrRawBodies(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Accounts[0].DeviceID = device.ID
	_, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.NoError(t, err, "a parsing model is not required")
	polled, err := PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	execution := polled["job"].(MonitorExecution)
	assert.Equal(t, "upstream-secret", execution.Account.Password)
	_, err = PollMonitorDevice(device, job.ID, "wrong")
	require.Error(t, err)
	require.Error(t, MonitorRecordEvent(device, job.ID, MonitorJobEvent{LeaseToken: execution.LeaseToken, Type: "complete", Complete: true}))
	require.Error(t, MonitorRecordEvent(device, job.ID, MonitorJobEvent{LeaseToken: execution.LeaseToken, Type: "progress", Step: 2001}))
	require.NoError(t, MonitorRecordEvent(device, job.ID, MonitorJobEvent{LeaseToken: execution.LeaseToken, Type: "progress", Step: 2000}))
	request := MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: monitorTestBatch(execution)}
	request.Result.Data.Groups[0].DisplayName = "  商家分组  "
	request.Result.Data.Groups = append(request.Result.Data.Groups,
		MonitorGroup{Name: "second", DisplayName: "商家分组", Ratio: request.Result.Data.Groups[0].Ratio},
		MonitorGroup{Name: "code-only", Ratio: request.Result.Data.Groups[0].Ratio},
	)
	other := *device
	other.ID = "other-browser"
	_, err = SubmitMonitorBrowserResult(&other, job.ID, request)
	require.Error(t, err)
	wrongLease := request
	wrongLease.LeaseToken = "wrong"
	_, err = SubmitMonitorBrowserResult(device, job.ID, wrongLease)
	require.Error(t, err)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", device.OwnerID).Update("role", common.RoleCommonUser).Error)
	_, err = SubmitMonitorBrowserResult(device, job.ID, request)
	require.Error(t, err)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", device.OwnerID).Update("role", common.RoleRootUser).Error)
	result, err := SubmitMonitorBrowserResult(device, job.ID, request)
	require.NoError(t, err)
	assert.Equal(t, true, result["confirmed"])
	assert.Equal(t, true, result["complete"])
	var snapshot model.MonitorSnapshot
	require.NoError(t, db.First(&snapshot, "job_id = ?", job.ID).Error)
	var data MonitorData
	require.NoError(t, common.UnmarshalJsonStr(string(snapshot.Data), &data))
	assert.Equal(t, 1, data.SchemaVersion)
	assert.Len(t, data.Sources, 4)
	assert.Equal(t, 60.0, *data.Balances[0].Amount)
	require.Len(t, data.Groups, 3)
	assert.Equal(t, "standard", data.Groups[0].Name)
	assert.Equal(t, "商家分组", data.Groups[0].DisplayName)
	assert.Equal(t, "second", data.Groups[1].Name)
	assert.Equal(t, "商家分组", data.Groups[1].DisplayName)
	assert.Equal(t, "code-only", data.Groups[2].Name)
	assert.Empty(t, data.Groups[2].DisplayName)
	assert.Equal(t, "standard", data.Models[0].Group)
	assert.NotContains(t, string(snapshot.Data), "do-not-store")
	assert.NotContains(t, string(snapshot.Data), "upstream-secret")
	assert.False(t, db.Migrator().HasTable("monitor_capture_records"))
	var stored model.MonitorJob
	require.NoError(t, db.First(&stored, "id = ?", job.ID).Error)
	assert.Equal(t, "completed", stored.Status)
	assert.True(t, stored.Complete)
	assert.Empty(t, stored.LeaseHash)
	_, err = SubmitMonitorBrowserResult(device, job.ID, request)
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&model.MonitorSnapshot{}).Where("job_id = ?", job.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestMonitorBrowserSignedBalancesPersistWithoutRounding(t *testing.T) {
	for _, tc := range []struct {
		name   string
		amount float64
	}{
		{"negative Sub2API balance", -0.00022783},
		{"zero balance", 0},
		{"negative limit", -1e15},
		{"positive limit", 1e15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := monitorTestDB(t)
			device := monitorTestDevice(t)
			config := monitorTestConfig()
			config.Accounts[0].DeviceID = device.ID
			config.Sites[0].Currency = "USD"
			config.Flows[0].Extraction.Adapter = "sub2api"
			config.Flows[0].Extraction.Mappings = []MonitorMapping{}
			_, err := SaveMonitorConfig(config)
			require.NoError(t, err)
			job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
			require.NoError(t, err)
			polled, err := PollMonitorDevice(device, "", "")
			require.NoError(t, err)
			execution := polled["job"].(MonitorExecution)
			batch := monitorTestBatch(execution)
			batch.Data.Balances[0].Amount = &tc.amount
			batch.Data.Balances[0].Currency = "USD"
			batch.Data.Models[0].Currency = "USD"
			result, err := SubmitMonitorBrowserResult(device, job.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: batch})
			require.NoError(t, err)
			assert.Equal(t, true, result["complete"])
			assert.Equal(t, true, result["confirmed"])
			returned := result["data"].(MonitorData)
			assert.Equal(t, batch.Data.Balances, returned.Balances)
			var snapshot model.MonitorSnapshot
			require.NoError(t, db.First(&snapshot, "job_id = ?", job.ID).Error)
			assert.True(t, snapshot.Complete)
			assert.True(t, snapshot.Confirmed)
			var stored MonitorData
			require.NoError(t, common.UnmarshalJsonStr(string(snapshot.Data), &stored))
			assert.Equal(t, batch.Data.Balances, stored.Balances)
			var completed model.MonitorJob
			require.NoError(t, db.First(&completed, "id = ?", job.ID).Error)
			assert.Equal(t, "completed", completed.Status)
			assert.True(t, completed.Complete)
		})
	}

	t.Run("invalid values do not persist", func(t *testing.T) {
		db := monitorTestDB(t)
		device := monitorTestDevice(t)
		config := monitorTestConfig()
		config.Accounts[0].DeviceID = device.ID
		_, err := SaveMonitorConfig(config)
		require.NoError(t, err)
		job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
		require.NoError(t, err)
		polled, err := PollMonitorDevice(device, "", "")
		require.NoError(t, err)
		execution := polled["job"].(MonitorExecution)
		invalidBalance := "Invalid monitor structured result"
		negativePrice := "Monitor values must be finite and non-negative"
		for _, tc := range []struct {
			name      string
			change    func(*MonitorBatchResult)
			wantError string
		}{
			{"NaN balance", func(r *MonitorBatchResult) { *r.Data.Balances[0].Amount = math.NaN() }, invalidBalance},
			{"positive infinite balance", func(r *MonitorBatchResult) { *r.Data.Balances[0].Amount = math.Inf(1) }, invalidBalance},
			{"negative infinite balance", func(r *MonitorBatchResult) { *r.Data.Balances[0].Amount = math.Inf(-1) }, invalidBalance},
			{"balance above limit", func(r *MonitorBatchResult) { *r.Data.Balances[0].Amount = math.Nextafter(1e15, math.Inf(1)) }, invalidBalance},
			{"balance below limit", func(r *MonitorBatchResult) { *r.Data.Balances[0].Amount = math.Nextafter(-1e15, math.Inf(-1)) }, invalidBalance},
			{"negative input price", func(r *MonitorBatchResult) { *r.Data.Models[0].Input = -1 }, negativePrice},
			{"negative output price", func(r *MonitorBatchResult) { *r.Data.Models[0].Output = -1 }, negativePrice},
			{"negative cache read price", func(r *MonitorBatchResult) { r.Data.Models[0].CacheRead = new(-1.0) }, negativePrice},
			{"negative cache write price", func(r *MonitorBatchResult) { r.Data.Models[0].CacheWrite = new(-1.0) }, negativePrice},
			{"negative request price", func(r *MonitorBatchResult) { r.Data.Models[0].RequestPrice = new(-1.0) }, negativePrice},
			{"negative group ratio", func(r *MonitorBatchResult) { *r.Data.Groups[0].Ratio = -1 }, negativePrice},
			{"negative token unit", func(r *MonitorBatchResult) { *r.Data.Models[0].TokenUnit = -1 }, "Token unit must be positive"},
			{"negative topup paid", func(r *MonitorBatchResult) {
				r.Data.Topups = []MonitorTopup{{Reference: "topup", Paid: new(-1.0), Currency: "CNY", FeeBearer: "other"}}
			}, negativePrice},
			{"negative topup credited", func(r *MonitorBatchResult) {
				r.Data.Topups = []MonitorTopup{{Reference: "topup", Credited: new(-1.0), Currency: "CNY", FeeBearer: "other"}}
			}, negativePrice},
			{"negative topup fee", func(r *MonitorBatchResult) {
				r.Data.Topups = []MonitorTopup{{Reference: "topup", Fee: new(-1.0), Currency: "CNY", FeeBearer: "other"}}
			}, negativePrice},
		} {
			t.Run(tc.name, func(t *testing.T) {
				batch := monitorTestBatch(execution)
				*batch.Data.Balances[0].Amount = -0.00022783
				tc.change(&batch)
				_, err := SubmitMonitorBrowserResult(device, job.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: batch})
				require.EqualError(t, err, tc.wantError)
				var count int64
				require.NoError(t, db.Model(&model.MonitorSnapshot{}).Where("job_id = ?", job.ID).Count(&count).Error)
				assert.Zero(t, count)
				var running model.MonitorJob
				require.NoError(t, db.First(&running, "id = ?", job.ID).Error)
				assert.Equal(t, "running", running.Status)
				assert.False(t, running.Complete)
				assert.NotEmpty(t, running.LeaseHash)
			})
		}
	})
}

func TestMonitorBillingRulesSavePreviewRetainAndCollect(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Accounts[0].Password = "billing-passphrase-123"
	config.Accounts[0].DeviceID = device.ID
	_, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", FlowID: "flow", Mode: "record"})
	require.NoError(t, err)
	polled, err := PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	execution := polled["job"].(MonitorExecution)
	mapping := &execution.Flow.Extraction.Mappings[3]
	delete(mapping.Fields, "billing_unit")
	mapping.Billing = monitorTestBillingRules()
	mapping.Tiers = &MonitorTierMapping{Path: "/tiers", Mode: "whole_request", Basis: "context", Fields: map[string]MonitorFieldMapping{
		"input_threshold": {Kind: "path", Path: "/threshold"}, "input": {Kind: "path", Path: "/input"}, "output": {Kind: "path", Path: "/output"},
	}}
	preview := monitorTestBatch(execution)
	preview.Data.Models[0].Tiers = []MonitorPriceTier{{InputThreshold: new(int64(128000)), Input: new(4.0), Output: new(8.0)}}
	preview.Data.Models[0].TierMode, preview.Data.Models[0].TierBasis = "whole_request", "context"
	preview.Data.Models = append(preview.Data.Models,
		MonitorModelPrice{Name: "per-request", Group: "standard", Currency: "CNY", BillingUnit: "request", RequestPrice: new(0.025), IncludesGroup: new(false)},
		MonitorModelPrice{Name: "per-second", Group: "standard", Currency: "CNY", BillingUnit: "unsupported", Notes: "USD per second"},
	)
	request := MonitorFlowSaveRequest{LeaseToken: execution.LeaseToken, Flow: execution.Flow}
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.EqualError(t, err, "Preview the complete structured result before saving mappings")
	request.Preview = &preview
	for _, tc := range []struct{ name, value string }{
		{"notes", "USD billing-passphrase-123 per second"},
		{"condition path", "/billing%2Dpassphrase-123/input"},
		{"price path", "/billing-passphrase-123/request_price"},
	} {
		t.Run("credential in "+tc.name, func(t *testing.T) {
			rules := monitorTestBillingRules()
			switch tc.name {
			case "notes":
				rules.Rules[2].Notes = tc.value
			case "condition path":
				rules.Rules[0].Conditions[0].Path = tc.value
			case "price path":
				rules.Rules[1].RequestPrice.Path = tc.value
			}
			mapping.Billing = rules
			_, err := SaveMonitorBrowserFlow(device, job.ID, request)
			require.EqualError(t, err, "Recorded passwords must use a password variable")
			loaded, err := GetMonitorConfig()
			require.NoError(t, err)
			delete(loaded.Flows[0].Extraction.Mappings[3].Fields, "billing_unit")
			loaded.Flows[0].Extraction.Mappings[3].Billing = rules
			_, err = SaveMonitorConfig(loaded)
			require.EqualError(t, err, "Recorded passwords must use a password variable")
		})
	}
	mapping.Billing = monitorTestBillingRules()
	badPreview := preview
	badPreview.Sources = preview.Sources[1:]
	request.Preview = &badPreview
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.Error(t, err, "billing rules cannot bypass source completeness")
	badPreview = preview
	badPreview.Data.AccountID = "another-account"
	_, err = SaveMonitorBrowserFlow(device, job.ID, request)
	require.EqualError(t, err, "Captured account identity does not match the configured account")
	request.Preview = &preview
	encoded, err := common.Marshal(request)
	require.NoError(t, err)
	var apiRequest MonitorFlowSaveRequest
	require.NoError(t, common.DecodeJsonBodyStrict(strings.NewReader(string(encoded)), &apiRequest))
	saved, err := SaveMonitorBrowserFlow(device, job.ID, apiRequest)
	require.NoError(t, err)
	assert.Equal(t, mapping.Billing, saved.Extraction.Mappings[3].Billing)
	loaded, err := GetMonitorConfig()
	require.NoError(t, err)
	assert.Equal(t, saved, loaded.Flows[0])
	_, err = SaveMonitorConfig(loaded)
	require.NoError(t, err, "full configuration saves preserve reviewed billing rules")
	loaded, err = GetMonitorConfig()
	require.NoError(t, err)
	assert.Equal(t, saved.Extraction, loaded.Flows[0].Extraction)

	retained := MonitorFlowSaveRequest{LeaseToken: execution.LeaseToken, Flow: saved}
	retained.Flow.Name = "Page actions edited, classification unchanged"
	retained.Flow.Steps = []MonitorStep{{Action: "navigate", URL: "https://upstream.example/dashboard"}, {Action: "wait", TimeoutMS: 100}}
	saved, err = SaveMonitorBrowserFlow(device, job.ID, retained)
	require.NoError(t, err, "unchanged billing rules retain the verified preview")
	assert.Equal(t, mapping.Billing, saved.Extraction.Mappings[3].Billing)
	retained.Flow = saved
	retained.Flow.Extraction.Mappings[3].Billing.Rules[0].Conditions[0].Value = new(1.0)
	_, err = SaveMonitorBrowserFlow(device, job.ID, retained)
	require.EqualError(t, err, "Preview the complete structured result before saving mappings", "classification edits invalidate the previous preview")
	_, err = ControlMonitorJob(job.ID, "cancel")
	require.NoError(t, err)
	collect, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.NoError(t, err)
	polled, err = PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	execution = polled["job"].(MonitorExecution)
	assert.Equal(t, mapping.Billing, execution.Flow.Extraction.Mappings[3].Billing)
	batch := monitorTestBatch(execution)
	batch.Data.Models = preview.Data.Models
	_, err = SubmitMonitorBrowserResult(device, collect.ID, MonitorResultRequest{LeaseToken: "wrong", Result: batch})
	require.EqualError(t, err, "Invalid or expired monitor execution lease")
	result, err := SubmitMonitorBrowserResult(device, collect.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: batch})
	require.NoError(t, err)
	assert.Equal(t, true, result["complete"])
	assert.Equal(t, true, result["confirmed"])
	var snapshot model.MonitorSnapshot
	require.NoError(t, db.First(&snapshot, "job_id = ?", collect.ID).Error)
	var data MonitorData
	require.NoError(t, common.UnmarshalJsonStr(string(snapshot.Data), &data))
	assert.Equal(t, batch.Data.Models, data.Models, "the browser's structured token/request/unsupported output remains unchanged")
	assert.NotContains(t, string(snapshot.Data), `"billing":`)
}

func TestMonitorBrowserPricingRulesRoundTrip(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Accounts[0].DeviceID = device.ID
	config.Sites[0].Currency = "USD"
	mapping := &config.Flows[0].Extraction.Mappings[3]
	mapping.Fields["currency"] = MonitorFieldMapping{Kind: "constant", Value: common.RawMessage(`"USD"`)}
	mapping.Fields["billing_unit"] = MonitorFieldMapping{Kind: "path", Path: "/billing_unit"}
	mapping.Fields["billing_expression"] = MonitorFieldMapping{Kind: "path", Path: "/billing_expr"}
	mapping.Tiers = &MonitorTierMapping{Path: "/variants", Mode: "whole_request", Basis: "context", Fields: map[string]MonitorFieldMapping{
		"input_threshold": {Kind: "path", Path: "/threshold"},
		"input":           {Kind: "path", Path: "/input"},
		"output":          {Kind: "path", Path: "/output"},
	}}
	config.Flows[0].Extraction.Mappings[1].Fields["currency"] = MonitorFieldMapping{Kind: "constant", Value: common.RawMessage(`"USD"`)}
	_, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.NoError(t, err)
	polled, err := PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	execution := polled["job"].(MonitorExecution)
	frozen := execution.Flow.Extraction.Mappings[3]
	assert.Equal(t, mapping.Tiers, frozen.Tiers)
	assert.Equal(t, mapping.Fields["billing_expression"], frozen.Fields["billing_expression"])
	batch := monitorTestBatch(execution)
	batch.Data.Balances[0].Currency = "USD"
	structured := &batch.Data.Models[0]
	structured.Currency = "USD"
	structured.TierMode, structured.TierBasis = "whole_request", "context"
	structured.Tiers = []MonitorPriceTier{{InputThreshold: new(int64(272_000)), Input: new(4.0), Output: new(18.0)}}
	native := MonitorModelPrice{Name: "native", Group: "standard", Currency: "USD", BillingUnit: "expression", IncludesGroup: new(false), BillingExpression: `tier("native", fixed(0.05))`}
	pending := *structured
	pending.Name, pending.PricingPending = "pending", true
	pending.Tiers = []MonitorPriceTier{{InputThreshold: new(int64(272_000))}}
	batch.Data.Models = append(batch.Data.Models, native, pending)
	request := MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: batch}
	result, err := SubmitMonitorBrowserResult(device, job.ID, request)
	require.NoError(t, err)
	assert.Equal(t, true, result["confirmed"])
	assert.Equal(t, true, result["complete"])
	var snapshot model.MonitorSnapshot
	require.NoError(t, db.First(&snapshot, "job_id = ?", job.ID).Error)
	var data MonitorData
	require.NoError(t, common.UnmarshalJsonStr(string(snapshot.Data), &data))
	assert.Equal(t, batch.Data.Models, data.Models)
	_, _, err = monitorUpstreamPrice(data.Models[2], MonitorScenario{InputTokens: 1, Requests: 1})
	require.EqualError(t, err, "Upstream pricing rules need confirmation")
}

func TestMonitorBatchRejectsInvalidSourcesUnitsNumbersAndIncompleteClaims(t *testing.T) {
	config := monitorTestConfig()
	execution := MonitorExecution{Site: config.Sites[0], Account: config.Accounts[0], Flow: config.Flows[0]}
	cases := []struct {
		name   string
		change func(*MonitorBatchResult)
	}{
		{"mapping revision", func(r *MonitorBatchResult) { r.FlowRevision++ }},
		{"wrong account", func(r *MonitorBatchResult) { r.Data.AccountID = "43" }},
		{"unmapped response", func(r *MonitorBatchResult) { r.Sources[0].RequestID = "unknown" }},
		{"cross origin", func(r *MonitorBatchResult) { r.Sources[0].URL = "https://other.example/self" }},
		{"page origin", func(r *MonitorBatchResult) { r.Sources[0].PageURL = "https://other.example/dashboard" }},
		{"query retained", func(r *MonitorBatchResult) { r.Sources[0].PageURL += "?token=secret" }},
		{"stale response", func(r *MonitorBatchResult) { r.Sources[0].CapturedAt = 1 }},
		{"negative cost", func(r *MonitorBatchResult) { *r.Data.Models[0].Input = -1 }},
		{"not finite", func(r *MonitorBatchResult) { *r.Data.Models[0].Output = math.NaN() }},
		{"zero token unit", func(r *MonitorBatchResult) { *r.Data.Models[0].TokenUnit = 0 }},
		{"unknown billing unit", func(r *MonitorBatchResult) { r.Data.Models[0].BillingUnit = "credits" }},
		{"currency mismatch", func(r *MonitorBatchResult) { r.Data.Models[0].Currency = "USD" }},
		{"duplicate model", func(r *MonitorBatchResult) { r.Data.Models = append(r.Data.Models, r.Data.Models[0]) }},
		{"overlong group display name", func(r *MonitorBatchResult) { r.Data.Groups[0].DisplayName = strings.Repeat("商", 171) }},
		{"control in group display name", func(r *MonitorBatchResult) { r.Data.Groups[0].DisplayName = "group\nname" }},
		{"redacted group display name", func(r *MonitorBatchResult) { r.Data.Groups[0].DisplayName = "[REDACTED]" }},
		{"missing selected response", func(r *MonitorBatchResult) { r.Sources = r.Sources[1:] }},
		{"missing balance", func(r *MonitorBatchResult) { r.Data.Balances[0].Amount = nil }},
		{"explicit missing fields", func(r *MonitorBatchResult) { r.Missing = []string{"models.output"} }},
		{"credential in result", func(r *MonitorBatchResult) { r.Data.Warnings = []string{"upstream-secret"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := monitorTestBatch(execution)
			tc.change(&result)
			_, err := validateMonitorBatch(result, execution, common.GetTimestamp())
			require.Error(t, err)
		})
	}
}

func TestMonitorUnsupportedBillingRemainsExplicitWithoutInventedPrices(t *testing.T) {
	config := monitorTestConfig()
	execution := MonitorExecution{Site: config.Sites[0], Account: config.Accounts[0], Flow: config.Flows[0]}
	batch := monitorTestBatch(execution)
	price := &batch.Data.Models[0]
	price.BillingUnit, price.TokenUnit, price.Input, price.Output, price.IncludesGroup = "unsupported", nil, nil, nil, nil
	batch.Data.Warnings = []string{"Unsupported upstream billing rule"}
	data, err := validateMonitorBatch(batch, execution, common.GetTimestamp())
	require.NoError(t, err)
	assert.Equal(t, "unsupported", data.Models[0].BillingUnit)
	assert.Nil(t, data.Models[0].Input)
	assert.Nil(t, data.Models[0].IncludesGroup)
}

func TestMonitorPartialResultKeepsUnknownPricesAndCannotEnterProfit(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Accounts[0].DeviceID = device.ID
	_, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.NoError(t, err)
	polled, err := PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	execution := polled["job"].(MonitorExecution)
	batch := monitorTestBatch(execution)
	batch.Complete, batch.Data.Models[0].Output, batch.Missing = false, nil, []string{"models.output"}
	result, err := SubmitMonitorBrowserResult(device, job.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: batch})
	require.NoError(t, err)
	assert.Equal(t, false, result["confirmed"])
	assert.Equal(t, false, result["complete"])
	data := result["data"].(MonitorData)
	assert.Nil(t, data.Models[0].Output)
	assert.Equal(t, []string{"models.output"}, data.Missing)
	var eligible int64
	require.NoError(t, db.Model(&model.MonitorSnapshot{}).Where("account_id = ? AND confirmed = ? AND complete = ?", "account", true, true).Count(&eligible).Error)
	assert.Zero(t, eligible)
}

func TestMonitorExtractionRejectsUnsafeOrIncompleteRecipes(t *testing.T) {
	cases := []struct {
		name   string
		change func(*MonitorExtraction)
	}{
		{"identity constant", func(e *MonitorExtraction) {
			e.Mappings[0].Fields["value"] = MonitorFieldMapping{Kind: "constant", Value: common.RawMessage(`"42"`)}
		}},
		{"cross origin request", func(e *MonitorExtraction) { e.Requests[0].URL = "https://other.example/self" }},
		{"sensitive query", func(e *MonitorExtraction) { e.Requests[0].Query = map[string]string{"token": "secret"} }},
		{"credential source page", func(e *MonitorExtraction) {
			e.Requests[0].PageURLs = []string{"https://upstream.example/callback?code=one"}
		}},
		{"duplicate source page", func(e *MonitorExtraction) {
			e.Requests[0].PageURLs = []string{"https://upstream.example/dashboard", "https://upstream.example/dashboard"}
		}},
		{"bad pointer", func(e *MonitorExtraction) {
			e.Mappings[1].Fields["amount"] = MonitorFieldMapping{Kind: "path", Path: "/data/~2quota"}
		}},
		{"unsupported field", func(e *MonitorExtraction) {
			e.Mappings[2].Fields["password"] = MonitorFieldMapping{Kind: "path", Path: "/password"}
		}},
		{"fallback outside group identity", func(e *MonitorExtraction) {
			e.Mappings[2].Fields["ratio"] = MonitorFieldMapping{Kind: "path", Path: "/ratio", FallbackPath: "/group_name"}
		}},
		{"fallback on a flat model group", func(e *MonitorExtraction) {
			e.Mappings[3].Fields["group"] = MonitorFieldMapping{Kind: "path", Path: "/group_code", FallbackPath: "/group_name"}
		}},
		{"fallback without a primary path", func(e *MonitorExtraction) {
			e.Mappings[2].Fields["name"] = MonitorFieldMapping{Kind: "path", FallbackPath: "/group_name"}
		}},
		{"sensitive primary fallback path", func(e *MonitorExtraction) {
			e.Mappings[2].Fields["name"] = MonitorFieldMapping{Kind: "path", Path: "/password", FallbackPath: "/group_name"}
		}},
		{"invalid fallback path", func(e *MonitorExtraction) {
			e.Mappings[2].Fields["name"] = MonitorFieldMapping{Kind: "path", Path: "/group_code", FallbackPath: "/~2group_name"}
		}},
		{"cross-response fallback", func(e *MonitorExtraction) {
			e.Mappings[2].Fields["name"] = MonitorFieldMapping{Kind: "path", Path: "/group_code", FallbackPath: "/group_name", Source: "self"}
		}},
		{"fallback with arithmetic", func(e *MonitorExtraction) {
			e.Mappings[2].Fields["name"] = MonitorFieldMapping{Kind: "path", Path: "/group_code", FallbackPath: "/group_name", Divide: &MonitorFieldMapping{Kind: "path", Path: "/group_ratio"}}
		}},
		{"deep arithmetic", func(e *MonitorExtraction) {
			e.Mappings[1].Fields["amount"] = MonitorFieldMapping{Kind: "path", Multiply: &MonitorFieldMapping{Kind: "path", Multiply: &MonitorFieldMapping{Kind: "path", Divide: &MonitorFieldMapping{Kind: "path"}}}}
		}},
		{"billing expression translation", func(e *MonitorExtraction) {
			e.Mappings[3].Fields["billing_expression"] = MonitorFieldMapping{Kind: "path", Path: "/billing_expr", Translate: map[string]string{"captured": `tier("invented", fixed(0.01))`}}
		}},
		{"missing required group target", func(e *MonitorExtraction) { e.Mappings = e.Mappings[:2] }},
		{"automatic billing", func(e *MonitorExtraction) { e.Mappings[3].Fields["billing_unit"] = MonitorFieldMapping{Kind: "auto"} }},
		{"expanded item", func(e *MonitorExtraction) { e.Mappings[3].Fields["group"] = MonitorFieldMapping{Kind: "item"} }},
		{"keyed cross-source path", func(e *MonitorExtraction) {
			e.Mappings[2].Fields["ratio"] = MonitorFieldMapping{Kind: "path", Source: "groups", Path: "/ratios/{key}"}
		}},
		{"unknown adapter", func(e *MonitorExtraction) { e.Adapter = "other"; e.Mappings = []MonitorMapping{} }},
		{"adapter with mappings", func(e *MonitorExtraction) { e.Adapter = "newapi" }},
		{"adapter without mappings array", func(e *MonitorExtraction) { e.Adapter = "newapi"; e.Mappings = nil }},
		{"adapter without requests", func(e *MonitorExtraction) { e.Adapter = "sub2api"; e.Mappings = []MonitorMapping{}; e.Requests = nil }},
		{"nested models without a child path", func(e *MonitorExtraction) {
			e.Mappings[3].Iterate = "nested_array"
		}},
		{"credential-bearing child path", func(e *MonitorExtraction) {
			e.Mappings[3].Iterate, e.Mappings[3].ChildRoot = "nested_array", "/access_token"
		}},
		{"child path on a flat mapping", func(e *MonitorExtraction) {
			e.Mappings[3].ChildRoot = "/models"
		}},
		{"nested traversal for a non-model target", func(e *MonitorExtraction) {
			e.Mappings[2].Iterate, e.Mappings[2].ChildRoot = "nested_array", "/models"
		}},
		{"parent operand on a flat model mapping", func(e *MonitorExtraction) {
			e.Mappings[3].Fields["input"] = MonitorFieldMapping{Kind: "path", Path: "/input", Multiply: &MonitorFieldMapping{Kind: "parent", Path: "/group_ratio"}}
		}},
		{"parent field with an external source", func(e *MonitorExtraction) {
			e.Mappings[3].Iterate, e.Mappings[3].ChildRoot = "nested_array", "/models"
			e.Mappings[3].Fields["group"] = MonitorFieldMapping{Kind: "parent", Path: "/group_name", Source: "groups"}
		}},
		{"parent field without a path", func(e *MonitorExtraction) {
			e.Mappings[3].Iterate, e.Mappings[3].ChildRoot = "nested_array", "/models"
			e.Mappings[3].Fields["group"] = MonitorFieldMapping{Kind: "parent"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := monitorTestConfig()
			tc.change(config.Flows[0].Extraction)
			require.Error(t, validateMonitorExtraction(config.Flows[0].Extraction, config.Sites[0]))
		})
	}
}

func TestMonitorNestedModelMappingKeepsParentScopeBounded(t *testing.T) {
	cases := []struct {
		name   string
		valid  bool
		change func(*MonitorMapping)
	}{
		{"parent group and multiplier", true, func(mapping *MonitorMapping) {
			mapping.Fields["input"] = MonitorFieldMapping{Kind: "path", Path: "/base_input", Multiply: &MonitorFieldMapping{Kind: "parent", Path: "/group_ratio"}}
		}},
		{"malformed child pointer", false, func(mapping *MonitorMapping) { mapping.ChildRoot = "/models/~2" }},
		{"prototype child pointer", false, func(mapping *MonitorMapping) { mapping.ChildRoot = "/__proto__/models" }},
		{"deep child pointer", false, func(mapping *MonitorMapping) { mapping.ChildRoot = strings.Repeat("/models", 41) }},
		{"parent constant value", false, func(mapping *MonitorMapping) {
			mapping.Fields["group"] = MonitorFieldMapping{Kind: "parent", Path: "/group_name", Value: common.RawMessage(`"standard"`)}
		}},
		{"parent non-pointer path", false, func(mapping *MonitorMapping) {
			mapping.Fields["group"] = MonitorFieldMapping{Kind: "parent", Path: "group_name"}
		}},
		{"parent credential path", false, func(mapping *MonitorMapping) {
			mapping.Fields["group"] = MonitorFieldMapping{Kind: "parent", Path: "/credentials/name"}
		}},
		{"parent arithmetic beyond limit", false, func(mapping *MonitorMapping) {
			mapping.Fields["input"] = MonitorFieldMapping{Kind: "path", Multiply: &MonitorFieldMapping{Kind: "parent", Path: "/group_ratio", Multiply: &MonitorFieldMapping{Kind: "parent", Path: "/group_ratio", Multiply: &MonitorFieldMapping{Kind: "parent", Path: "/group_ratio"}}}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := monitorTestConfig()
			mapping := &config.Flows[0].Extraction.Mappings[3]
			mapping.Root, mapping.Iterate, mapping.ChildRoot = "/data/items", "nested_array", "/models"
			mapping.Fields["group"] = MonitorFieldMapping{Kind: "parent", Path: "/group_name"}
			tc.change(mapping)
			err := validateMonitorExtraction(config.Flows[0].Extraction, config.Sites[0])
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestMonitorResultRequiresAllConfiguredSourcePages(t *testing.T) {
	config := monitorTestConfig()
	execution := MonitorExecution{Site: config.Sites[0], Account: config.Accounts[0], Flow: config.Flows[0]}
	rule := &execution.Flow.Extraction.Requests[0]
	rule.PageURLs = []string{"https://upstream.example/dashboard", "https://upstream.example/dashboard?page=2"}
	batch := monitorTestBatch(execution)
	_, err := validateMonitorBatch(batch, execution, common.GetTimestamp())
	require.Error(t, err, "a response from the first page cannot stand in for the second page")
	source := batch.Sources[0]
	source.PageURL = rule.PageURLs[1]
	batch.Sources = append(batch.Sources, source)
	_, err = validateMonitorBatch(batch, execution, common.GetTimestamp())
	require.NoError(t, err)
	batch.Sources[0].PageURL = "https://upstream.example/other-page"
	_, err = validateMonitorBatch(batch, execution, common.GetTimestamp())
	require.Error(t, err)
}

func TestMonitorResultRequiresExactUniqueModelGroupBindings(t *testing.T) {
	config := monitorTestConfig()
	execution := MonitorExecution{Site: config.Sites[0], Account: config.Accounts[0], Flow: config.Flows[0]}
	batch := monitorTestBatch(execution)
	batch.Data.Models[0].Group = " standard "
	_, err := validateMonitorBatch(batch, execution, common.GetTimestamp())
	require.EqualError(t, err, "Invalid model identity, group or currency")
	batch = monitorTestBatch(execution)
	batch.Data.Models = append(batch.Data.Models, batch.Data.Models[0])
	_, err = validateMonitorBatch(batch, execution, common.GetTimestamp())
	require.EqualError(t, err, "Invalid model identity, group or currency")
}

func TestMonitorCollectionPagesKeepSafePaginationAndRejectCredentials(t *testing.T) {
	for _, adapter := range []string{"", "newapi", "sub2api"} {
		t.Run("adapter="+adapter, func(t *testing.T) {
			config := monitorTestConfig()
			extraction := config.Flows[0].Extraction
			extraction.Adapter = adapter
			if adapter != "" {
				extraction.Mappings = []MonitorMapping{}
			}
			valid := extraction.Requests[0]
			valid.PageURLs = []string{"https://upstream.example/dashboard?page=2"}
			valid.Query = map[string]string{"page": "2"}
			extraction.Requests[0] = valid
			require.NoError(t, validateMonitorExtraction(extraction, config.Sites[0]))
			assert.Equal(t, valid, extraction.Requests[0])
			for _, invalid := range []MonitorRequestRule{
				{ID: valid.ID, Method: "GET", URL: valid.URL, PageURLs: []string{"https://upstream.example/callback?code=one"}},
				{ID: valid.ID, Method: "GET", URL: valid.URL, PageURLs: []string{"https://other.example/dashboard"}},
				{ID: valid.ID, Method: "GET", URL: "https://other.example/self", PageURLs: valid.PageURLs},
				{ID: valid.ID, Method: "GET", URL: valid.URL + "?token=secret", PageURLs: valid.PageURLs},
				{ID: valid.ID, Method: "GET", URL: valid.URL, Query: map[string]string{"token": "secret"}, PageURLs: valid.PageURLs},
			} {
				extraction.Requests[0] = invalid
				require.Error(t, validateMonitorExtraction(extraction, config.Sites[0]))
			}
		})
	}
}

func TestMonitorExpiredLeaseRequiresExplicitRetryAndDeviceSerializesJobs(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Accounts[0].DeviceID = device.ID
	saved, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	first, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.NoError(t, err)
	second, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.NoError(t, err)
	saved.Accounts[0].Password = "new-password"
	_, err = SaveMonitorConfig(saved)
	require.NoError(t, err)
	result, err := PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	execution := result["job"].(MonitorExecution)
	assert.Equal(t, "upstream-secret", execution.Account.Password)
	encoded, err := common.Marshal(first)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "upstream-secret")
	result, err = PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	assert.Nil(t, result["job"])
	require.NoError(t, db.Model(&model.MonitorJob{}).Where("id = ?", execution.ID).Update("lease_until", common.GetTimestamp()-1).Error)
	result, err = PollMonitorDevice(device, execution.ID, execution.LeaseToken)
	require.NoError(t, err)
	assert.Equal(t, "failed", result["status"])
	_, err = SubmitMonitorBrowserResult(device, execution.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: monitorTestBatch(execution)})
	require.Error(t, err)
	result, err = PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	next := result["job"].(MonitorExecution)
	assert.NotEqual(t, execution.ID, next.ID)
	assert.Contains(t, []string{first.ID, second.ID}, next.ID)
}

func TestMonitorRestartInterruptsExactExecutionsWithoutClaimOrReplay(t *testing.T) {
	for _, tc := range []struct{ mode, status string }{{"record", "running"}, {"collect", "awaiting_user"}} {
		t.Run(tc.mode, func(t *testing.T) {
			db := monitorTestDB(t)
			device := monitorTestDevice(t)
			config := monitorTestConfig()
			config.Accounts[0].DeviceID = device.ID
			_, err := SaveMonitorConfig(config)
			require.NoError(t, err)
			job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", FlowID: "flow", Mode: tc.mode})
			require.NoError(t, err)
			polled, err := PollMonitorDevice(device, "", "")
			require.NoError(t, err)
			execution := polled["job"].(MonitorExecution)
			require.Equal(t, job.ID, execution.ID)
			if tc.status == "awaiting_user" {
				require.NoError(t, MonitorRecordEvent(device, job.ID, MonitorJobEvent{Type: "awaiting_user", LeaseToken: execution.LeaseToken}))
			}
			queued, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", FlowID: "flow", Mode: tc.mode})
			require.NoError(t, err)
			other := *device
			other.ID, other.TokenHash, other.ActiveJob = "other-browser", monitorHash("other-token"), "other-job"
			require.NoError(t, db.Create(&other).Error)
			preserved := []model.MonitorJob{
				{ID: "other-job", DeviceID: other.ID, Status: "running", LeaseHash: "other-lease", LeaseUntil: common.GetTimestamp() + 120},
				{ID: "completed-job", DeviceID: device.ID, Status: "completed", Complete: true, Message: "Saved", UpdatedAt: 12},
				{ID: "cancelled-job", DeviceID: device.ID, Status: "cancelled", Control: "cancel", Message: "Stopped", UpdatedAt: 13},
				*queued,
			}
			require.NoError(t, db.Create(preserved[:3]).Error)
			// Active executions must be found even if transient device-slot metadata is absent.
			orphan := model.MonitorJob{ID: "orphan-job", DeviceID: device.ID, Status: "awaiting_user", LeaseHash: "orphan-lease", LeaseUntil: common.GetTimestamp() + 120, Frozen: "encrypted-inputs"}
			require.NoError(t, db.Create(&orphan).Error)
			var before []model.MonitorJob
			require.NoError(t, db.Order("id").Find(&before).Error)
			var deviceBefore model.MonitorDevice
			require.NoError(t, db.First(&deviceBefore, "id = ?", device.ID).Error)
			discovered, err := RestartMonitorDevice(device, nil)
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{job.ID, orphan.ID}, discovered["active_job_ids"])
			var after []model.MonitorJob
			require.NoError(t, db.Order("id").Find(&after).Error)
			assert.Equal(t, before, after, "discovery cannot modify or claim jobs")
			var currentDevice model.MonitorDevice
			require.NoError(t, db.First(&currentDevice, "id = ?", device.ID).Error)
			assert.Equal(t, deviceBefore, currentDevice)

			ids := []string{job.ID, orphan.ID, other.ActiveJob, "completed-job", "cancelled-job", queued.ID, "missing-job"}
			ack, err := RestartMonitorDevice(device, ids)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"interrupted": int64(2)}, ack)
			for _, id := range []string{job.ID, orphan.ID} {
				var stopped model.MonitorJob
				require.NoError(t, db.First(&stopped, "id = ?", id).Error)
				assert.Equal(t, "failed", stopped.Status)
				assert.Equal(t, "Browser executor restarted; interrupted task was stopped", stopped.Message)
				assert.Equal(t, "cancel", stopped.Control)
				assert.False(t, stopped.Complete)
				assert.Empty(t, stopped.LeaseHash)
				assert.Zero(t, stopped.LeaseUntil)
				assert.Empty(t, stopped.Frozen)
			}
			for _, expected := range preserved {
				var stored model.MonitorJob
				require.NoError(t, db.First(&stored, "id = ?", expected.ID).Error)
				assert.Equal(t, expected, stored)
			}
			require.NoError(t, db.First(&currentDevice, "id = ?", device.ID).Error)
			assert.Empty(t, currentDevice.ActiveJob)
			var storedOther model.MonitorDevice
			require.NoError(t, db.First(&storedOther, "id = ?", other.ID).Error)
			assert.Equal(t, other, storedOther)
			require.NoError(t, db.Order("id").Find(&before).Error)
			ack, err = RestartMonitorDevice(device, ids)
			require.NoError(t, err)
			assert.Equal(t, int64(0), ack["interrupted"])
			require.NoError(t, db.Order("id").Find(&after).Error)
			assert.Equal(t, before, after)
			require.Error(t, MonitorRecordEvent(device, job.ID, MonitorJobEvent{Type: "progress", Message: "stale", LeaseToken: execution.LeaseToken}))
			if tc.mode == "record" {
				preview := monitorTestBatch(execution)
				_, err = SaveMonitorBrowserFlow(device, job.ID, MonitorFlowSaveRequest{LeaseToken: execution.LeaseToken, Flow: execution.Flow, Preview: &preview})
			} else {
				_, err = SubmitMonitorBrowserResult(device, job.ID, MonitorResultRequest{LeaseToken: execution.LeaseToken, Result: monitorTestBatch(execution)})
			}
			require.Error(t, err, "the previous lease cannot save after interruption")
			polled, err = PollMonitorDevice(device, job.ID, execution.LeaseToken)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"job": nil, "control": "cancel", "status": "failed"}, polled)
			unchanged, err := GetMonitorConfig()
			require.NoError(t, err)
			assert.Equal(t, config.Flows[0].Revision, unchanged.Flows[0].Revision)
			var snapshots int64
			require.NoError(t, db.Model(&model.MonitorSnapshot{}).Count(&snapshots).Error)
			assert.Zero(t, snapshots)

			polled, err = PollMonitorDevice(device, "", "")
			require.NoError(t, err)
			next := polled["job"].(MonitorExecution)
			assert.Equal(t, queued.ID, next.ID, "queued work starts only on a subsequent poll")
			ack, err = RestartMonitorDevice(device, []string{job.ID, orphan.ID})
			require.NoError(t, err)
			assert.Equal(t, int64(0), ack["interrupted"], "a delayed exact confirmation cannot interrupt the new job")
			_, err = PollMonitorDevice(device, next.ID, next.LeaseToken)
			require.NoError(t, err)
			ack, err = RestartMonitorDevice(device, []string{})
			require.NoError(t, err)
			assert.Equal(t, int64(0), ack["interrupted"], "an empty confirmation never becomes discovery or a broad update")
			var active model.MonitorJob
			require.NoError(t, db.First(&active, "id = ?", next.ID).Error)
			assert.Equal(t, "running", active.Status)
			require.NoError(t, db.First(&currentDevice, "id = ?", device.ID).Error)
			assert.Equal(t, next.ID, currentDevice.ActiveJob)
		})
	}
}

func TestMonitorRestartRechecksDeviceAndRootAuthorization(t *testing.T) {
	for _, tc := range []string{"revoked", "expired", "rotated-token", "changed-owner", "disabled-root", "demoted-root", "deleted-root"} {
		t.Run(tc, func(t *testing.T) {
			db := monitorTestDB(t)
			device := monitorTestDevice(t)
			job := model.MonitorJob{ID: "active-job", DeviceID: device.ID, Status: "running", LeaseHash: "old-lease", LeaseUntil: common.GetTimestamp() + 120}
			require.NoError(t, db.Create(&job).Error)
			require.NoError(t, db.Model(&model.MonitorDevice{}).Where("id = ?", device.ID).Update("active_job", job.ID).Error)
			switch tc {
			case "revoked":
				require.NoError(t, db.Model(&model.MonitorDevice{}).Where("id = ?", device.ID).Update("revoked_at", common.GetTimestamp()).Error)
			case "expired":
				require.NoError(t, db.Model(&model.MonitorDevice{}).Where("id = ?", device.ID).Update("expires_at", common.GetTimestamp()-1).Error)
			case "rotated-token":
				require.NoError(t, db.Model(&model.MonitorDevice{}).Where("id = ?", device.ID).Update("token_hash", monitorHash("replacement-token")).Error)
			case "changed-owner":
				require.NoError(t, db.Model(&model.MonitorDevice{}).Where("id = ?", device.ID).Update("owner_id", device.OwnerID+1).Error)
			case "disabled-root":
				require.NoError(t, db.Model(&model.User{}).Where("id = ?", device.OwnerID).Update("status", common.UserStatusDisabled).Error)
			case "demoted-root":
				require.NoError(t, db.Model(&model.User{}).Where("id = ?", device.OwnerID).Update("role", common.RoleCommonUser).Error)
			case "deleted-root":
				require.NoError(t, db.Delete(&model.User{}, device.OwnerID).Error)
			}
			_, err := RestartMonitorDevice(device, nil)
			require.Error(t, err, "discovery must recheck authorization after middleware authentication")
			_, err = RestartMonitorDevice(device, []string{job.ID})
			require.Error(t, err, "confirmation must recheck authorization after middleware authentication")
			var stored model.MonitorJob
			require.NoError(t, db.First(&stored, "id = ?", job.ID).Error)
			assert.Equal(t, job, stored)
			var current model.MonitorDevice
			require.NoError(t, db.First(&current, "id = ?", device.ID).Error)
			assert.Equal(t, job.ID, current.ActiveJob)
		})
	}
}

func TestMonitorRestartRequiresExplicitRetryAndBoundsIDs(t *testing.T) {
	db := monitorTestDB(t)
	device := monitorTestDevice(t)
	config := monitorTestConfig()
	config.Accounts[0].DeviceID, config.Accounts[0].IntervalMinutes = device.ID, 1
	_, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	job, err := CreateMonitorJob(MonitorJobRequest{AccountID: "account", Mode: "collect"})
	require.NoError(t, err)
	_, err = PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.MonitorJob{}).Where("id = ?", job.ID).Update("created_at", common.GetTimestamp()-3600).Error)
	for _, ids := range [][]string{{job.ID, job.ID}, {""}, {strings.Repeat("x", 65)}, make([]string, 101)} {
		_, err := RestartMonitorDevice(device, ids)
		require.EqualError(t, err, "Invalid monitor request")
	}
	discovered, err := RestartMonitorDevice(device, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{job.ID}, discovered["active_job_ids"])
	ack, err := RestartMonitorDevice(device, []string{job.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), ack["interrupted"])
	require.NoError(t, runMonitorScheduler(context.Background()))
	var count int64
	require.NoError(t, db.Model(&model.MonitorJob{}).Count(&count).Error)
	assert.Equal(t, int64(1), count, "the scheduler cannot automatically retry or log in after executor interruption")
	polled, err := PollMonitorDevice(device, "", "")
	require.NoError(t, err)
	assert.Nil(t, polled["job"])
	discovered, err = RestartMonitorDevice(device, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"active_job_ids": []string{}}, discovered)
	retried, err := ControlMonitorJob(job.ID, "retry")
	require.NoError(t, err)
	assert.NotEqual(t, job.ID, retried.ID)
	assert.Equal(t, "queued", retried.Status)
}

func TestMonitorUnboundCoverageAndConfirmedSnapshotOutsideHistoryLimit(t *testing.T) {
	db := monitorTestDB(t)
	channel := model.Channel{Name: "Bound", Key: "test", Status: common.ChannelStatusEnabled, Models: "monitor-priced", Group: "default"}
	require.NoError(t, db.Create(&channel).Error)
	other := model.Channel{Name: "Unbound", Key: "test", Status: common.ChannelStatusEnabled, Models: "monitor-missing", Group: "default"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&[]model.Ability{{Group: "default", Model: "monitor-priced", ChannelId: channel.Id, Enabled: true}, {Group: "default", Model: "monitor-missing", ChannelId: other.Id, Enabled: true}}).Error)
	require.NoError(t, db.Create(&[]model.Option{{Key: "billing_setting.billing_mode", Value: `{"monitor-priced":"tiered_expr"}`}, {Key: "billing_setting.billing_expr", Value: `{"monitor-priced":"len <= 200000 ? tier(\"standard\", p * 5 + c * 10) : tier(\"long\", p * 4 + c * 15)"}`}}).Error)
	config := monitorTestConfig()
	config.Scenario.InputTokens = 1_000_000
	config.Scenario.OutputTokens = 1_000
	config.Scenario.USDExchange = 1
	config.Bindings = []MonitorBinding{{ID: "binding", ChannelID: channel.Id, AccountID: "account", UpstreamGroup: "standard"}}
	_, err := SaveMonitorConfig(config)
	require.NoError(t, err)
	price, unit, ratio, include := 2.0, 1000000.0, 1.0, true
	data := MonitorData{AccountID: "42", Groups: []MonitorGroup{{Name: "standard", Ratio: &ratio}}, Models: []MonitorModelPrice{{Name: "monitor-priced", Group: "standard", Currency: "CNY", BillingUnit: "token", TokenUnit: &unit, Input: &price, Output: &price, IncludesGroup: &include, TierMode: "whole_request", TierBasis: "context", Tiers: []MonitorPriceTier{{InputThreshold: new(int64(272_000)), Input: new(5.0), Output: &price}}}}}
	encoded, err := common.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.MonitorSnapshot{ID: "confirmed-old", AccountID: "account", Confirmed: true, Complete: true, Data: model.LongText(encoded), CreatedAt: 1}).Error)
	pending := make([]model.MonitorSnapshot, 201)
	for i := range pending {
		pending[i] = model.MonitorSnapshot{ID: monitorID("snap_"), AccountID: "account", Complete: true, Data: model.LongText(encoded), CreatedAt: 100}
	}
	require.NoError(t, db.Create(&pending).Error)
	state, err := MonitorState()
	require.NoError(t, err)
	assert.Len(t, state["snapshots"], 200)
	rows := state["profit_rows"].([]MonitorProfitRow)
	require.Len(t, rows, 2)
	selection := MonitorProfitSelection{BindingID: "binding", LocalGroup: "default", LocalModel: "monitor-priced"}
	detail, err := GetMonitorProfitDetail(selection)
	require.NoError(t, err)
	assert.Equal(t, "confirmed-old", detail.Row.SnapshotID)
	assert.Equal(t, "binding", detail.Row.BindingID)
	selection.InputTokens = new(0)
	preview, err := GetMonitorProfitDetail(selection)
	require.NoError(t, err)
	assert.Zero(t, preview.ContextTokens)
	assert.Nil(t, preview.PerMillionContext)
	saved, err := GetMonitorConfig()
	require.NoError(t, err)
	assert.Equal(t, config.Scenario.InputTokens, saved.Scenario.InputTokens)
	selection.LocalModel = "monitor-missing"
	_, err = GetMonitorProfitDetail(selection)
	require.EqualError(t, err, "Procurement route is no longer available")
	for _, row := range rows {
		if row.ChannelID == channel.Id {
			assert.Equal(t, "ready", row.Status)
			assert.Equal(t, "confirmed-old", row.SnapshotID)
			require.Len(t, row.TierSamples, 3)
			for i, want := range []struct {
				input           int
				local, upstream string
				cost            float64
			}{
				{200_000, "standard", "base", 0.402},
				{200_001, "long", "base", 0.402002},
				{1_000_000, "long", "context > 272000", 5.002},
			} {
				sample := row.TierSamples[i]
				assert.Equal(t, want.input, sample.ContextTokens)
				assert.Equal(t, want.local, sample.LocalTier)
				assert.Equal(t, want.upstream, sample.UpstreamTier)
				require.NotNil(t, sample.Cost)
				assert.InDelta(t, want.cost, *sample.Cost, 1e-12)
				assert.Empty(t, sample.Components)
			}
			assert.Equal(t, row.Revenue, row.TierSamples[2].Revenue)
			assert.Equal(t, row.Profit, row.TierSamples[2].Profit)
			assert.Equal(t, row.Margin, row.TierSamples[2].Margin)
		} else {
			assert.Equal(t, "pending", row.Status)
			assert.Empty(t, row.AccountID)
			assert.Nil(t, row.Cost)
		}
	}
}

func TestMonitorProfitFeesAndUnknownCosts(t *testing.T) {
	input, output, tokenUnit, ratio, included := 2.0, 6.0, 1000000.0, 0.5, false
	price := MonitorModelPrice{Currency: "CNY", BillingUnit: "token", TokenUnit: &tokenUnit, Input: &input, Output: &output, IncludesGroup: &included}
	entry := model.ModelPricingEntry{ModelName: "test", ModelPricingDescription: model.ModelPricingDescription{Effective: model.PricingValues{"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `tier("base", p * 4 + c * 12)`}}}
	scenario := MonitorScenario{InputTokens: 1000000, OutputTokens: 1000000, Requests: 1, USDExchange: 1, RevenueFactor: 1, FeeBearer: "self", FeeRate: 0.02}
	account := MonitorAccount{PurchaseFactor: 1, FeeBearer: "self", FeeRate: 0.01}
	row := MonitorProfitRow{LocalRatio: 0.085, UpstreamRatio: &ratio, Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, MonitorSite{Currency: "CNY", FXRate: 1}, price, entry)
	require.Equal(t, "ready", row.Status)
	require.NotNil(t, row.Margin)
	assert.InDelta(t, 1.3328, *row.Revenue, 1e-9)
	assert.InDelta(t, 4.04, *row.Cost, 1e-9)
	assert.InDelta(t, -2.7072, *row.Profit, 1e-9)
	assert.InDelta(t, -2.7072/1.3328, *row.Margin, 1e-9)
	account.FeeBearer = "unknown"
	row = MonitorProfitRow{LocalRatio: 0.085, UpstreamRatio: &ratio, Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, MonitorSite{Currency: "CNY", FXRate: 1}, price, entry)
	assert.Nil(t, row.Profit)
	account.FeeBearer = "other"
	price.Output = nil
	monitorCalculateProfit(&row, scenario, account, MonitorSite{Currency: "CNY", FXRate: 1}, price, entry)
	assert.Nil(t, row.Cost)
}

func TestMonitorWholeRequestUsesCompleteContextAndRepricesEveryCategory(t *testing.T) {
	price := MonitorModelPrice{
		BillingUnit: "token", TokenUnit: new(1_000_000.0), Input: new(0.02), Output: new(0.12), CacheRead: new(0.002), CacheWrite: new(0.025),
		TierMode: "whole_request", TierBasis: "context",
		Tiers: []MonitorPriceTier{{InputThreshold: new(int64(272_000)), Input: new(0.04), Output: new(0.18), CacheRead: new(0.004), CacheWrite: new(0.05)}},
	}
	for _, tc := range []struct {
		input int
		label string
		cost  float64
	}{
		{31_999, "base", 0.00323998},
		{32_000, "base", 0.00324},
		{32_001, "context > 272000", 0.00588004},
	} {
		scenario := MonitorScenario{InputTokens: tc.input, OutputTokens: 10_000, CacheRead: 200_000, CacheWrite: 40_000, Requests: 2}
		cost, label, err := monitorUpstreamPrice(price, scenario)
		require.NoError(t, err)
		assert.Equal(t, tc.label, label)
		assert.InDelta(t, 2*tc.cost, cost, 1e-12)
	}
	price.Tiers[0].CacheRead = nil
	_, _, err := monitorUpstreamPrice(price, MonitorScenario{InputTokens: 32_001, CacheRead: 240_000, Requests: 1})
	require.EqualError(t, err, "A required upstream unit price is unknown")
}

func TestMonitorGraduatedChargesOnlyUncachedInputByBracket(t *testing.T) {
	price := MonitorModelPrice{
		BillingUnit: "token", TokenUnit: new(1_000.0), Input: new(1.0), Output: new(3.0), CacheRead: new(0.5),
		TierMode: "graduated", TierBasis: "input",
		Tiers: []MonitorPriceTier{{InputThreshold: new(int64(1_000)), Input: new(0.0)}, {InputThreshold: new(int64(2_000)), Input: new(2.0)}},
	}
	for _, tc := range []struct {
		input int
		label string
		cost  float64
	}{{1_000, "base", 2.25}, {2_000, "input > 1000", 2.25}, {2_500, "input > 2000", 3.25}} {
		cost, label, err := monitorUpstreamPrice(price, MonitorScenario{InputTokens: tc.input, OutputTokens: 250, CacheRead: 1_000, Requests: 1})
		require.NoError(t, err)
		assert.Equal(t, tc.label, label)
		assert.InDelta(t, tc.cost, cost, 1e-12)
	}
	price.Tiers[0].Input = nil
	_, _, err := monitorUpstreamPrice(price, MonitorScenario{InputTokens: 1_001, Requests: 1})
	require.EqualError(t, err, "A required upstream unit price is unknown")
	_, _, err = monitorUpstreamPrice(price, MonitorScenario{InputTokens: -1, Requests: 1})
	require.EqualError(t, err, "Invalid scenario token count")
}

func TestMonitorProfitUsesNativeExpressionTiersOnBothSides(t *testing.T) {
	price := MonitorModelPrice{Currency: "USD", BillingUnit: "expression", IncludesGroup: new(false), BillingExpression: `v1:len <= 272000 ? tier("fixed", fixed(0.01)) : tier("long", p * 4 + c * 18 + cr * 0.4 + cc * 5)`}
	local := model.ModelPricingEntry{ModelName: "test", ModelPricingDescription: model.ModelPricingDescription{Effective: model.PricingValues{
		"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `len <= 272000 ? tier("local-base", p * 2) : tier("local-long", p * 7)`,
	}}}
	scenario := MonitorScenario{InputTokens: 32_000, OutputTokens: 10_000, CacheRead: 200_000, CacheWrite: 40_000, Requests: 2, USDExchange: 7, RevenueFactor: 1, FeeBearer: "other"}
	account := MonitorAccount{PurchaseFactor: 1, FeeBearer: "other"}
	site := MonitorSite{Currency: "USD", FXRate: 7}
	row := MonitorProfitRow{LocalRatio: 1, UpstreamRatio: new(0.5), Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	require.Equal(t, "ready", row.Status)
	assert.Equal(t, "fixed", row.UpstreamTier)
	assert.Equal(t, "local-base", row.LocalTier)
	assert.InDelta(t, 0.07, *row.Cost, 1e-12)
	assert.InDelta(t, 7.616, *row.Revenue, 1e-12)

	scenario.InputTokens++
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	require.Equal(t, "ready", row.Status)
	assert.Equal(t, "long", row.UpstreamTier)
	assert.Equal(t, "local-long", row.LocalTier)
	assert.InDelta(t, 4.116028, *row.Cost, 1e-12)
	assert.InDelta(t, 26.656098, *row.Revenue, 1e-12)

	price.PricingPending = true
	row = MonitorProfitRow{LocalRatio: 1, UpstreamRatio: new(0.5), Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	assert.Equal(t, "Upstream pricing rules need confirmation", row.Reason)
	assert.Nil(t, row.Cost)
}

func TestMonitorCapturedExpressionKeepsCurrencyAndMatchesPurchaseAndSaleTiers(t *testing.T) {
	const expression = `v1:len <= 272000 ? tier("standard", p * 8 + c * 40 + cr * 0.4 + cc * 10) : tier("synthetic-long", p * 16 + c * 60 + cr * 0.8 + cc * 20)`
	config := monitorTestConfig()
	mapping := &config.Flows[0].Extraction.Mappings[3]
	mapping.Fields["billing_unit"] = MonitorFieldMapping{Kind: "path", Path: "/billing_unit"}
	mapping.Fields["billing_expression"] = MonitorFieldMapping{Kind: "path", Path: "/billing_expr"}
	execution := MonitorExecution{Site: config.Sites[0], Account: config.Accounts[0], Flow: config.Flows[0]}
	batch := monitorTestBatch(execution)
	batch.Data.Groups[0].Ratio = new(0.11)
	batch.Data.Models = []MonitorModelPrice{{Name: "gpt-6.2-astra", Group: "standard", Currency: "CNY", BillingUnit: "expression", IncludesGroup: new(false), BillingExpression: expression}}
	data, err := validateMonitorBatch(batch, execution, common.GetTimestamp())
	require.NoError(t, err)
	price := data.Models[0]
	assert.Equal(t, "CNY", price.Currency)
	assert.Equal(t, expression, price.BillingExpression)
	local := model.ModelPricingEntry{ModelName: price.Name, ModelPricingDescription: model.ModelPricingDescription{Effective: model.PricingValues{
		"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `len <= 300000 ? tier("sale-standard", p * 2 + c * 12 + cr * 0.2 + cc * 3) : tier("sale-long", p * 4 + c * 18 + cr * 0.4 + cc * 5)`,
	}}}
	scenario := MonitorScenario{Currency: "CNY", OutputTokens: 10_000, CacheRead: 200_000, CacheWrite: 40_000, Requests: 2, USDExchange: 7, RevenueFactor: 1, FeeBearer: "other"}
	account, site := execution.Account, execution.Site
	for _, tc := range []struct {
		name                    string
		input                   int
		purchaseTier, saleTier  string
		baseCost, cost, revenue float64
		unitPrices              [4]float64
	}{
		{"below boundary", 31_999, "standard", "sale-standard", 2.271984, 0.24991824, 2.407986, [4]float64{0.88, 4.4, 0.044, 1.1}},
		{"at boundary", 32_000, "standard", "sale-standard", 2.272, 0.24992, 2.408, [4]float64{0.88, 4.4, 0.044, 1.1}},
		{"purchase changes tier", 32_001, "synthetic-long", "sale-standard", 4.144032, 0.45584352, 2.408014, [4]float64{1.76, 6.6, 0.088, 2.2}},
		{"sale changes tier independently", 60_001, "synthetic-long", "sale-long", 5.040032, 0.55440352, 4.900028, [4]float64{1.76, 6.6, 0.088, 2.2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scenario.InputTokens = tc.input
			cost, tier, err := monitorUpstreamPrice(price, scenario)
			require.NoError(t, err)
			assert.Equal(t, tc.purchaseTier, tier)
			assert.InDelta(t, tc.baseCost, cost, 1e-12)
			row := MonitorProfitRow{LocalRatio: 0.5, UpstreamRatio: data.Groups[0].Ratio, Status: "pending"}
			monitorCalculateProfit(&row, scenario, account, site, price, local)
			require.Equal(t, "ready", row.Status, row.Reason)
			assert.Equal(t, tc.purchaseTier, row.UpstreamTier)
			assert.Equal(t, tc.saleTier, row.LocalTier)
			assert.InDelta(t, tc.cost, *row.Cost, 1e-12)
			assert.InDelta(t, tc.revenue, *row.Revenue, 1e-12)
			assert.InDelta(t, tc.revenue-tc.cost, *row.Profit, 1e-12)
			detail := monitorProfitScenarioDetail(row, scenario, account, site, &price, local.Effective["billing_setting.billing_expr"].(string))
			assert.Equal(t, "CNY", detail.Upstream.Currency)
			assert.Equal(t, "USD", detail.Local.Currency)
			assert.Equal(t, expression, detail.Upstream.Expression)
			require.Len(t, detail.Components, 4)
			for i, component := range detail.Components {
				require.NotNil(t, component.UpstreamUnitPrice, component.Kind)
				assert.InDelta(t, tc.unitPrices[i], *component.UpstreamUnitPrice, 1e-12, component.Kind)
			}
		})
	}

	// Each side converts its own currency into the configured reporting currency.
	scenario.InputTokens, scenario.Currency, scenario.USDExchange = 32_000, "USD", 1
	site.FXRate = 1.0 / 7
	for _, includesGroup := range []bool{false, true} {
		price.IncludesGroup = new(includesGroup)
		if includesGroup {
			price.BillingExpression = "v1:(" + strings.TrimPrefix(expression, "v1:") + ") * 0.11"
		}
		row := MonitorProfitRow{LocalRatio: 0.5, UpstreamRatio: data.Groups[0].Ratio, Status: "pending"}
		monitorCalculateProfit(&row, scenario, account, site, price, local)
		require.Equal(t, "ready", row.Status, row.Reason)
		assert.InDelta(t, 0.24992/7, *row.Cost, 1e-12, "includes_group=%t", includesGroup)
		assert.InDelta(t, 0.344, *row.Revenue, 1e-12)
	}
	price.Currency = "USD"
	row := MonitorProfitRow{LocalRatio: 0.5, UpstreamRatio: data.Groups[0].Ratio, Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	assert.Equal(t, "pending", row.Status)
	assert.Equal(t, "Confirm upstream price currency and whether it includes the group ratio", row.Reason)
	assert.Nil(t, row.Cost)
}

func TestMonitorProfitDetailsReconcileEveryComponentAndContextUnit(t *testing.T) {
	price := MonitorModelPrice{Currency: "USD", BillingUnit: "token", TokenUnit: new(1_000.0), IncludesGroup: new(false), Input: new(2.0), Output: new(6.0), CacheRead: new(0.2), CacheWrite: new(2.5), TierMode: "whole_request", TierBasis: "context", Tiers: []MonitorPriceTier{{InputThreshold: new(int64(272_000)), Input: new(4.0), Output: new(18.0), CacheRead: new(0.4), CacheWrite: new(5.0)}}}
	local := model.ModelPricingEntry{ModelName: "test", ModelPricingDescription: model.ModelPricingDescription{Effective: model.PricingValues{"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `len <= 272000 ? tier("standard", p * 6 + c * 20 + cr * 0.5 + cc * 8) : tier("long", p * 12 + c * 30 + cr * 1 + cc * 16)`}}}
	scenario := MonitorScenario{Currency: "CNY", InputTokens: 32_001, OutputTokens: 10_000, CacheRead: 200_000, CacheWrite: 40_000, Requests: 3, USDExchange: 7, RevenueFactor: 0.9, FeeBearer: "self", FeeRate: 0.02}
	account := MonitorAccount{PurchaseFactor: 0.8, FeeBearer: "self", FeeRate: 0.03}
	site := MonitorSite{Currency: "USD", FXRate: 7}
	row := MonitorProfitRow{LocalRatio: 2, UpstreamRatio: new(0.01), Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	require.Equal(t, "ready", row.Status)
	detail := monitorExplainProfit(row, scenario, account, site, &price, local)
	require.Len(t, detail.Components, 4)
	require.NotNil(t, detail.Local.Amount)
	require.NotNil(t, detail.Upstream.Amount)
	assert.Equal(t, row.Revenue, detail.Local.Amount)
	assert.Equal(t, row.Cost, detail.Upstream.Amount)
	assert.Equal(t, 272_001, detail.ContextTokens)
	require.Len(t, detail.TierSamples, 2)
	low, high := detail.TierSamples[0], detail.TierSamples[1]
	assert.Equal(t, 32_000, low.InputTokens)
	assert.Equal(t, 272_000, low.ContextTokens)
	assert.Equal(t, "standard", low.LocalTier)
	assert.Equal(t, "base", low.UpstreamTier)
	require.NotNil(t, low.Profit)
	require.Len(t, low.Components, 4)
	require.Len(t, high.Components, 4)
	assert.InDelta(t, *low.Revenue-*low.Cost, *low.Profit, 1e-12)
	lowRevenue, lowCost := 0.0, 0.0
	for _, component := range low.Components {
		require.NotNil(t, component.Revenue)
		require.NotNil(t, component.Cost)
		lowRevenue += *component.Revenue
		lowCost += *component.Cost
	}
	assert.InDelta(t, *low.Revenue, lowRevenue, 1e-10)
	assert.InDelta(t, *low.Cost, lowCost, 1e-10)
	assert.InDelta(t, 2**low.Components[0].LocalUnitPrice, *high.Components[0].LocalUnitPrice, 1e-10)
	assert.InDelta(t, 2**low.Components[0].UpstreamUnitPrice, *high.Components[0].UpstreamUnitPrice, 1e-10)
	assert.Equal(t, 272_001, high.ContextTokens)
	assert.Equal(t, row.LocalTier, high.LocalTier)
	assert.Equal(t, row.UpstreamTier, high.UpstreamTier)
	assert.Equal(t, row.Profit, high.Profit)
	assert.Equal(t, row.Margin, high.Margin)
	assert.Equal(t, "long", detail.Local.Tier)
	assert.Equal(t, "context > 272000", detail.Upstream.Tier)
	assert.Equal(t, 1_000.0, *detail.Upstream.TokenUnit)
	assert.Equal(t, 2.0, *detail.Upstream.BasePrices.Input)
	revenue, cost := 0.0, 0.0
	for _, part := range detail.Components {
		require.NotNil(t, part.Revenue, part.Kind)
		require.NotNil(t, part.Cost, part.Kind)
		require.NotNil(t, part.UnitProfit)
		revenue += *part.Revenue
		cost += *part.Cost
	}
	assert.InDelta(t, *row.Revenue, revenue, 1e-10)
	assert.InDelta(t, *row.Cost, cost, 1e-10)
	require.NotNil(t, detail.PerMillionContext)
	assert.InDelta(t, *row.Revenue*1_000_000/(272_001*3), detail.PerMillionContext.Revenue, 1e-10)
	_, err := common.Marshal(detail)
	require.NoError(t, err)

	price.Tiers[0].CacheRead = nil
	row = MonitorProfitRow{LocalRatio: 2, UpstreamRatio: new(0.01), Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	detail = monitorExplainProfit(row, scenario, account, site, &price, local)
	assert.Equal(t, "pending", detail.Row.Status)
	assert.Nil(t, detail.Upstream.Amount)
	assert.NotNil(t, detail.Components[0].UpstreamUnitPrice)
	assert.Nil(t, detail.Components[2].UpstreamUnitPrice)
	assert.Nil(t, detail.PerMillionContext)
	require.Len(t, detail.TierSamples, 2)
	require.Len(t, detail.TierSamples[1].Components, 4)
	assert.Nil(t, detail.TierSamples[1].Components[2].UpstreamUnitPrice)
	assert.Nil(t, detail.TierSamples[1].Components[2].Cost)
	assert.Nil(t, detail.TierSamples[1].Components[2].UnitMargin)
}

func TestMonitorProfitTierSamplesUseBothBoundariesAndKeepPendingAmountsUnknown(t *testing.T) {
	price := MonitorModelPrice{Currency: "USD", BillingUnit: "token", IncludesGroup: new(true), TokenUnit: new(1_000_000.0), Input: new(2.0), Output: new(10.0), TierMode: "whole_request", TierBasis: "context", Tiers: []MonitorPriceTier{{InputThreshold: new(int64(272_000)), Input: new(4.0), Output: new(15.0)}}}
	local := model.ModelPricingEntry{ModelName: "test", ModelPricingDescription: model.ModelPricingDescription{Effective: model.PricingValues{
		"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `200000 > len ? tier("standard", p * 5 + c * 10) : tier("long", p * 4 + c * 15)`,
	}}}
	scenario := MonitorScenario{InputTokens: 1_000_000, OutputTokens: 1_000, Requests: 1, USDExchange: 1, RevenueFactor: 1, FeeBearer: "other"}
	account := MonitorAccount{PurchaseFactor: 1, FeeBearer: "other"}
	site := MonitorSite{Currency: "USD", FXRate: 1}
	row := MonitorProfitRow{LocalRatio: 1, Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	detail := monitorExplainProfit(row, scenario, account, site, &price, local)
	require.Len(t, detail.TierSamples, 4)
	for i, expected := range []struct {
		context  int
		local    string
		upstream string
	}{
		{199_999, "standard", "base"}, {200_000, "long", "base"},
		{272_000, "long", "base"}, {272_001, "long", "context > 272000"},
	} {
		sample := detail.TierSamples[i]
		assert.Equal(t, expected.context, sample.ContextTokens)
		assert.Equal(t, expected.local, sample.LocalTier)
		assert.Equal(t, expected.upstream, sample.UpstreamTier)
		require.NotNil(t, sample.Revenue)
		require.NotNil(t, sample.Cost)
		require.NotNil(t, sample.Profit)
		assert.InDelta(t, *sample.Revenue-*sample.Cost, *sample.Profit, 1e-12)
	}
	assert.Equal(t, 1_000_000, detail.Scenario.InputTokens)
	assert.Equal(t, row.Profit, detail.Row.Profit)

	price.Tiers[0].Output = nil
	detail = monitorExplainProfit(row, scenario, account, site, &price, local)
	last := detail.TierSamples[len(detail.TierSamples)-1]
	assert.Equal(t, "pending", last.Status)
	assert.Nil(t, last.Cost)
	assert.Nil(t, last.Profit)
	assert.Nil(t, last.Margin)
	row = MonitorProfitRow{LocalRatio: 1, Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	expression, err := monitorLocalExpression(local)
	require.NoError(t, err)
	list := monitorProfitTierSamples(row, scenario, account, site, price, local, expression, false)
	require.Len(t, list, 3)
	assert.Equal(t, "ready", list[0].Status)
	assert.Equal(t, "pending", list[2].Status)
	assert.Nil(t, list[2].Revenue)
	assert.Nil(t, list[2].Cost)
	assert.Nil(t, list[2].Profit)
	assert.Nil(t, list[2].Margin)
	assert.Equal(t, "long", list[2].LocalTier)
	assert.Equal(t, "context > 272000", list[2].UpstreamTier)
}

func TestMonitorProfitComparesStandardCacheWritesWithLongTTLPricesConfigured(t *testing.T) {
	local := model.ModelPricingEntry{ModelName: "gpt-test", ModelPricingDescription: model.ModelPricingDescription{Effective: model.PricingValues{
		"billing_setting.billing_mode": "tiered_expr",
		"billing_setting.billing_expr": `len <= 272000 ? tier("standard", p * 2 + cc * 2.5 + cc12h * 7) : tier("long", p * 4 + cc * 5 + cc12h * 14)`,
	}}}
	price := MonitorModelPrice{Currency: "USD", BillingUnit: "token", IncludesGroup: new(true), TokenUnit: new(1_000_000.0), Input: new(1.0), Output: new(0.0), CacheWrite: new(2.0), TierMode: "whole_request", TierBasis: "context", Tiers: []MonitorPriceTier{{InputThreshold: new(int64(272_000)), Input: new(3.0), Output: new(0.0), CacheWrite: new(4.0)}}}
	account := MonitorAccount{PurchaseFactor: 1, FeeBearer: "other"}
	site := MonitorSite{Currency: "USD", FXRate: 1}
	scenario := MonitorScenario{Currency: "USD", InputTokens: 271_000, CacheWrite: 1_000, Requests: 1, USDExchange: 1, RevenueFactor: 1, FeeBearer: "other"}
	row := MonitorProfitRow{LocalRatio: 1, Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	require.Equal(t, "ready", row.Status)
	assert.InDelta(t, 0.5445, *row.Revenue, 1e-12)
	assert.InDelta(t, 0.273, *row.Cost, 1e-12)
	detail := monitorExplainProfit(row, scenario, account, site, &price, local)
	require.Len(t, detail.TierSamples, 2)
	for index, expected := range [][2]float64{{2.5, 2}, {5, 4}} {
		sample := detail.TierSamples[index]
		assert.Equal(t, "ready", sample.Status)
		require.NotNil(t, sample.Margin)
		require.Len(t, sample.Components, 4)
		write := sample.Components[3]
		assert.Equal(t, "cache_write", write.Kind)
		assert.Equal(t, 1_000, write.Quantity)
		require.NotNil(t, write.LocalUnitPrice)
		require.NotNil(t, write.UpstreamUnitPrice)
		assert.InDelta(t, expected[0], *write.LocalUnitPrice, 1e-12)
		assert.InDelta(t, expected[1], *write.UpstreamUnitPrice, 1e-12)
	}
	price.Tiers[0].CacheWrite = nil
	scenario.InputTokens++
	row = MonitorProfitRow{LocalRatio: 1, Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	assert.Equal(t, "pending", row.Status)
	assert.Nil(t, row.Margin)
	assert.Equal(t, "A required upstream unit price is unknown", row.Reason)
}

func TestMonitorExpressionDetailsPreserveConditionalAndNonlinearSemantics(t *testing.T) {
	scenario := MonitorScenario{InputTokens: 300_000, OutputTokens: 50_000, CacheRead: 100_000, CacheWrite: 10_000, Requests: 2}
	for _, tc := range []struct {
		expression string
		rates      []*float64
	}{
		{`tier("base", p * 2 + c * 8)`, []*float64{new(2.0), new(8.0), new(2.0), new(2.0)}},
		{`len > 272000 ? tier("long", p * 4 + c * 18 + cr * 0.4 + cc * 5) : tier("base", p * 2)`, []*float64{new(4.0), new(18.0), new(0.4), new(5.0)}},
		{`tier("base", p * 2) * 3`, []*float64{new(6.0), new(0.0), new(6.0), new(6.0)}},
		{`tier("nonlinear", p * c)`, []*float64{nil, nil, nil, nil}},
		{`tier("rounded", ceil(p / 1000) * 2)`, []*float64{nil, nil, nil, nil}},
		{`tier("context-rate", p * len)`, []*float64{nil, nil, nil, nil}},
	} {
		t.Run(tc.expression, func(t *testing.T) {
			amount, trace, err := monitorScenarioExpression(tc.expression, scenario, false)
			require.NoError(t, err)
			prices := monitorExpressionUnitPrices(tc.expression, scenario, trace, amount)
			for i, expected := range tc.rates {
				if expected == nil {
					assert.Nil(t, prices[i])
				} else {
					require.NotNil(t, prices[i])
					assert.InDelta(t, *expected, *prices[i], 1e-12)
				}
			}
		})
	}
}

func TestMonitorProfitDetailsSupportFixedAndGraduatedPrices(t *testing.T) {
	local := model.ModelPricingEntry{ModelName: "test", ModelPricingDescription: model.ModelPricingDescription{Effective: model.PricingValues{"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `tier("request", fixed(0.02))`}}}
	scenario := MonitorScenario{Requests: 3, USDExchange: 7, RevenueFactor: 1, FeeBearer: "other"}
	account := MonitorAccount{PurchaseFactor: 1, FeeBearer: "other"}
	site := MonitorSite{Currency: "USD", FXRate: 7}
	price := MonitorModelPrice{BillingUnit: "request", Currency: "USD", IncludesGroup: new(true), RequestPrice: new(0.01)}
	row := MonitorProfitRow{LocalRatio: 1, Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	detail := monitorExplainProfit(row, scenario, account, site, &price, local)
	require.Len(t, detail.Components, 5)
	assert.InDelta(t, 0.07, *detail.Components[4].UnitProfit, 1e-12)
	assert.InDelta(t, 0.5, *detail.Components[4].UnitMargin, 1e-12)
	assert.Nil(t, detail.PerMillionContext)

	mismatchedSite := MonitorSite{Currency: "CNY", FXRate: 1}
	row = MonitorProfitRow{LocalRatio: 1, Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, mismatchedSite, price, local)
	detail = monitorExplainProfit(row, scenario, account, mismatchedSite, &price, local)
	assert.Equal(t, "pending", detail.Row.Status)
	assert.Equal(t, "USD", detail.Upstream.Currency)
	assert.Nil(t, detail.Components[4].Cost)
	assert.Nil(t, detail.Components[4].UpstreamUnitPrice)

	unknownFee := scenario
	unknownFee.FeeBearer = "unknown"
	row = MonitorProfitRow{LocalRatio: 1, Status: "pending"}
	monitorCalculateProfit(&row, unknownFee, account, site, price, local)
	detail = monitorExplainProfit(row, unknownFee, account, site, &price, local)
	assert.Equal(t, "pending", detail.Row.Status)
	assert.Nil(t, detail.Components[4].Revenue)
	assert.Nil(t, detail.Components[4].LocalUnitPrice)

	price = MonitorModelPrice{BillingUnit: "token", Currency: "USD", TokenUnit: new(1_000.0), IncludesGroup: new(true), Input: new(1.0), Output: new(3.0), TierMode: "graduated", TierBasis: "input", Tiers: []MonitorPriceTier{{InputThreshold: new(int64(1_000)), Input: new(2.0)}}}
	scenario.InputTokens = 2_000
	row = MonitorProfitRow{LocalRatio: 1, Status: "pending"}
	monitorCalculateProfit(&row, scenario, account, site, price, local)
	detail = monitorExplainProfit(row, scenario, account, site, &price, local)
	assert.InDelta(t, 1_500*7, *detail.Components[0].UpstreamUnitPrice, 1e-10)
	assert.InDelta(t, *row.Cost, *detail.Components[0].Cost, 1e-10)
}

func TestMonitorProfitRejectsAnUnrepresentableMargin(t *testing.T) {
	price := MonitorModelPrice{Currency: "USD", BillingUnit: "expression", IncludesGroup: new(true), BillingExpression: `tier("request", fixed(1e300))`}
	local := model.ModelPricingEntry{ModelName: "test", ModelPricingDescription: model.ModelPricingDescription{Effective: model.PricingValues{
		"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `tier("request", fixed(1e-20))`,
	}}}
	scenario := MonitorScenario{Requests: 1, USDExchange: 1, RevenueFactor: 1, FeeBearer: "other"}
	row := MonitorProfitRow{LocalRatio: 1, Status: "pending"}
	monitorCalculateProfit(&row, scenario, MonitorAccount{PurchaseFactor: 1, FeeBearer: "other"}, MonitorSite{Currency: "USD", FXRate: 1}, price, local)
	assert.Equal(t, "pending", row.Status)
	assert.Equal(t, "Scenario amount is outside the supported range", row.Reason)
	assert.Nil(t, row.Margin)
}

func TestMonitorRejectsInvalidTierRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*MonitorModelPrice)
	}{
		{"missing threshold", func(p *MonitorModelPrice) { p.Tiers[0].InputThreshold = nil }},
		{"zero threshold", func(p *MonitorModelPrice) { p.Tiers[0].InputThreshold = new(int64(0)) }},
		{"threshold bound", func(p *MonitorModelPrice) { p.Tiers[0].InputThreshold = new(int64(1_000_000_001)) }},
		{"duplicate threshold", func(p *MonitorModelPrice) { p.Tiers = append(p.Tiers, p.Tiers[0]) }},
		{"unordered threshold", func(p *MonitorModelPrice) {
			p.Tiers = append(p.Tiers, MonitorPriceTier{InputThreshold: new(int64(10))})
		}},
		{"negative price", func(p *MonitorModelPrice) { p.Tiers[0].Input = new(-1.0) }},
		{"infinite price", func(p *MonitorModelPrice) { p.Tiers[0].Output = new(math.Inf(1)) }},
		{"too many tiers", func(p *MonitorModelPrice) {
			for i := range 17 {
				p.Tiers = append(p.Tiers, MonitorPriceTier{InputThreshold: new(int64(101 + i))})
			}
		}},
		{"mixed rules", func(p *MonitorModelPrice) { p.BillingExpression = `tier("base", p)` }},
		{"non token tiers", func(p *MonitorModelPrice) { p.BillingUnit = "request" }},
		{"graduated context", func(p *MonitorModelPrice) { p.TierMode = "graduated" }},
		{"graduated output", func(p *MonitorModelPrice) { p.TierMode, p.TierBasis = "graduated", "input" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			price := MonitorModelPrice{BillingUnit: "token", TierMode: "whole_request", TierBasis: "context", Tiers: []MonitorPriceTier{{InputThreshold: new(int64(100)), Input: new(2.0), Output: new(4.0)}}}
			tc.change(&price)
			require.EqualError(t, validateMonitorPriceTiers(price), "Invalid upstream price tiers")
		})
	}
	for _, expression := range []string{strings.Repeat("p+", 4096) + "p", `tier("base", p.map)`, `tier("base", [p][0])`, `tier("base", unknown(1))`} {
		require.EqualError(t, validateMonitorPriceExpression(expression), "Invalid upstream billing expression")
	}
	config := monitorTestConfig()
	mapping := &config.Flows[0].Extraction.Mappings[3]
	mapping.Tiers = &MonitorTierMapping{Path: "/variants", Mode: "whole_request", Basis: "context", Fields: map[string]MonitorFieldMapping{"input_threshold": {Kind: "path", Path: "/threshold"}, "input": {Kind: "path", Path: "/input"}}}
	require.NoError(t, validateMonitorExtraction(config.Flows[0].Extraction, config.Sites[0]))
	mapping.Tiers.Fields["input"] = MonitorFieldMapping{Kind: "path", Path: "/input", Scale: new(2.0)}
	require.EqualError(t, validateMonitorExtraction(config.Flows[0].Extraction, config.Sites[0]), "Invalid upstream price tiers")
}

func TestMonitorScenarioDoesNotAssumeMissingExpressionInputs(t *testing.T) {
	local := model.ModelPricingEntry{ModelName: "test", ModelPricingDescription: model.ModelPricingDescription{Effective: model.PricingValues{
		"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `tier("sale", p * 8)`,
	}}}
	scenario := MonitorScenario{InputTokens: 100, Requests: 1, USDExchange: 7, RevenueFactor: 1, FeeBearer: "other"}
	account, site := MonitorAccount{PurchaseFactor: 1, FeeBearer: "other"}, MonitorSite{Currency: "CNY", FXRate: 1}
	for _, expression := range []string{
		`true ? tier("text", p) : tier("other", ai)`,
		`true ? tier("text", p) : tier("other", img)`,
		`true ? tier("text", p) : tier("other", u("seconds"))`,
		`true ? tier("text", p) : tier("other", hour("UTC"))`,
		`tier("text", p) * (header("speed") == "fast" ? 2 : 1)`,
		`tier("text", p) * (param("service_tier") == "fast" ? 2 : 1)`,
		`tier("image", p * image_count)`,
	} {
		t.Run(expression, func(t *testing.T) {
			require.NoError(t, validateMonitorPriceExpression(expression))
			price := MonitorModelPrice{Currency: "CNY", BillingUnit: "expression", IncludesGroup: new(false), BillingExpression: expression, Input: new(8.0), Output: new(40.0)}
			_, _, err := monitorUpstreamPrice(price, scenario)
			require.EqualError(t, err, "Upstream billing needs unsupported scenario input")
			row := MonitorProfitRow{LocalRatio: 1, UpstreamRatio: new(0.11), Status: "pending"}
			monitorCalculateProfit(&row, scenario, account, site, price, local)
			assert.Equal(t, "pending", row.Status)
			assert.Equal(t, err.Error(), row.Reason)
			assert.Nil(t, row.Cost)
			assert.Nil(t, row.Profit)
		})
	}
	for _, expression := range []string{"", `tier("negative", -p)`, `tier("infinite", p / c)`} {
		price := MonitorModelPrice{Currency: "CNY", BillingUnit: "expression", IncludesGroup: new(true), BillingExpression: expression, Input: new(8.0), Output: new(40.0)}
		_, _, err := monitorUpstreamPrice(price, scenario)
		require.EqualError(t, err, "Upstream billing expression could not be evaluated")
		row := MonitorProfitRow{LocalRatio: 1, Status: "pending"}
		monitorCalculateProfit(&row, scenario, account, site, price, local)
		assert.Equal(t, "pending", row.Status)
		assert.Equal(t, err.Error(), row.Reason)
		assert.Nil(t, row.Cost)
		assert.Nil(t, row.Profit)
	}
}
