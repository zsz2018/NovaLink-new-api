package service

import (
	"bytes"
	"errors"

	"github.com/QuantumNous/new-api/common"
)

// MonitorConfig is the versioned, server-owned configuration. Passwords and API
// keys are write-only inputs; stored secrets are encrypted separately.
type MonitorConfig struct {
	Version  int64            `json:"version"`
	Sites    []MonitorSite    `json:"sites"`
	Accounts []MonitorAccount `json:"accounts"`
	Flows    []MonitorFlow    `json:"flows"`
	Bindings []MonitorBinding `json:"bindings"`
	Scenario MonitorScenario  `json:"scenario"`
}

type MonitorSite struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Origin         string   `json:"origin"`
	AllowedOrigins []string `json:"allowed_origins"`
	Currency       string   `json:"currency"`
	FXRate         float64  `json:"fx_rate"` // reporting currency per site currency
}

type MonitorAccount struct {
	ID               string                         `json:"id"`
	SiteID           string                         `json:"site_id"`
	Name             string                         `json:"name"`
	Username         string                         `json:"username"`
	ExternalID       string                         `json:"external_id"`
	Password         string                         `json:"password,omitempty"`
	HasPassword      bool                           `json:"has_password"`
	ClearPassword    bool                           `json:"clear_password,omitempty"`
	DeviceID         string                         `json:"device_id"`
	FlowID           string                         `json:"flow_id"`
	IntervalMinutes  int                            `json:"interval_minutes"` // zero disables scheduling
	Enabled          bool                           `json:"enabled"`
	PurchaseFactor   float64                        `json:"purchase_factor"` // real spend / credited face value
	FeeRate          float64                        `json:"fee_rate"`
	FeeBearer        string                         `json:"fee_bearer"` // self, other, unknown
	OfficialEstimate *MonitorOfficialEstimatePolicy `json:"official_estimate,omitempty"`
}

// Comparison policy belongs to this server-side account, never to a browser
// extraction. API and persisted configuration share the same strict boundary.
func (account *MonitorAccount) UnmarshalJSON(data []byte) error {
	type accountJSON MonitorAccount
	decoded := struct {
		*accountJSON
		OfficialEstimate common.RawMessage `json:"official_estimate"`
	}{accountJSON: &accountJSON{}}
	if err := common.DecodeJsonBodyStrict(bytes.NewReader(data), &decoded); err != nil {
		return err
	}
	if len(decoded.OfficialEstimate) != 0 {
		var value any
		if err := common.Unmarshal(decoded.OfficialEstimate, &value); err != nil {
			return err
		}
		if monitorBillingContainsNull(value) {
			return errors.New("Invalid monitor official estimate policy")
		}
		if err := common.DecodeJsonBodyStrict(bytes.NewReader(decoded.OfficialEstimate), &decoded.accountJSON.OfficialEstimate); err != nil {
			return err
		}
	}
	*account = MonitorAccount(*decoded.accountJSON)
	return nil
}

type MonitorStep struct {
	ID            string            `json:"id,omitempty"`
	Name          string            `json:"name,omitempty"`
	Action        string            `json:"action"`
	URL           string            `json:"url,omitempty"`
	Selector      string            `json:"selector,omitempty"`
	Text          string            `json:"text,omitempty"`
	Value         string            `json:"value,omitempty"`
	Variable      string            `json:"variable,omitempty"` // username, password; login inputs only
	TimeoutMS     int               `json:"timeout_ms,omitempty"`
	Group         string            `json:"group,omitempty"`
	RequestID     string            `json:"request_id,omitempty"`
	State         string            `json:"state,omitempty"`
	Condition     *MonitorCondition `json:"condition,omitempty"`
	Steps         []MonitorStep     `json:"steps,omitempty"`
	Otherwise     []MonitorStep     `json:"otherwise,omitempty"`
	MaxIterations int               `json:"max_iterations,omitempty"`
}

type MonitorCondition struct {
	Selector string `json:"selector,omitempty"`
	URL      string `json:"url,omitempty"`
	State    string `json:"state,omitempty"`
	Text     string `json:"text,omitempty"`
}

// MonitorLogin composes page actions with credential variables. Each credential
// input binds to its explicit URL or the login URL, including the exact path.
type MonitorLogin struct {
	URL       string            `json:"url"`
	Steps     []MonitorStep     `json:"steps"`
	TimeoutMS int               `json:"timeout_ms,omitempty"`
	Success   *MonitorCondition `json:"success,omitempty"`
}

type MonitorFlow struct {
	ID             string             `json:"id"`
	SiteID         string             `json:"site_id"`
	Name           string             `json:"name"`
	Revision       int64              `json:"revision"`
	Steps          []MonitorStep      `json:"steps"`
	CaptureSeconds int                `json:"capture_seconds"`
	Extraction     *MonitorExtraction `json:"extraction,omitempty"`
	Login          *MonitorLogin      `json:"login,omitempty"`
}

// Extraction is a server-owned, declarative recipe executed by the browser.
// Paths are RFC 6901 JSON pointers, never JavaScript or query expressions.
type MonitorExtraction struct {
	Version  int                  `json:"version"`
	Adapter  string               `json:"adapter,omitempty"`
	Requests []MonitorRequestRule `json:"requests"`
	Mappings []MonitorMapping     `json:"mappings"`
}

// The operator selects a bounded estimate scope, not upstream model support.
// Catalog versions are immutable and never advance with application updates.
type MonitorOfficialEstimatePolicy struct {
	CatalogVersion string                         `json:"catalog_version"`
	Models         []MonitorOfficialEstimateModel `json:"models"`
}

type MonitorOfficialEstimateModel struct {
	Name    string `json:"name"`
	Group   string `json:"group"`
	EntryID string `json:"entry_id"`
}

type MonitorOfficialCatalog struct {
	Version    string                        `json:"version"`
	VerifiedAt string                        `json:"verified_at"`
	Entries    []MonitorOfficialCatalogEntry `json:"entries"`
}

type MonitorOfficialCatalogEntry struct {
	ID         string            `json:"id"`
	Provider   string            `json:"provider"`
	Model      string            `json:"model"`
	SourceURL  string            `json:"source_url"`
	VerifiedAt string            `json:"verified_at"`
	Quote      MonitorModelPrice `json:"quote"`
}

type MonitorPriceSource struct {
	Kind           string `json:"kind"`
	CatalogVersion string `json:"catalog_version"`
	EntryID        string `json:"entry_id"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	SourceURL      string `json:"source_url"`
	VerifiedAt     string `json:"verified_at"`
}

// Extraction rules are strict at both API and persisted JSON boundaries. An
// omitted adapter selects generic mappings; only named browser adapters may be
// explicitly selected. Adapter parsing remains in the independent extension.
func (extraction *MonitorExtraction) UnmarshalJSON(data []byte) error {
	var decoded struct {
		Version  int                  `json:"version"`
		Adapter  common.RawMessage    `json:"adapter"`
		Requests []MonitorRequestRule `json:"requests"`
		Mappings []MonitorMapping     `json:"mappings"`
	}
	if err := common.DecodeJsonBodyStrict(bytes.NewReader(data), &decoded); err != nil {
		return err
	}
	var adapter string
	if len(decoded.Adapter) != 0 {
		if common.GetJsonType(decoded.Adapter) != "string" || common.Unmarshal(decoded.Adapter, &adapter) != nil || adapter != "newapi" && adapter != "sub2api" {
			return errors.New("Invalid monitor extraction rules")
		}
	}
	*extraction = MonitorExtraction{Version: decoded.Version, Adapter: adapter, Requests: decoded.Requests, Mappings: decoded.Mappings}
	return nil
}

type MonitorRequestRule struct {
	ID       string            `json:"id"`
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	Query    map[string]string `json:"query,omitempty"`
	PageURLs []string          `json:"page_urls,omitempty"`
}

type MonitorMapping struct {
	ID        string                         `json:"id"`
	RequestID string                         `json:"request_id"`
	Target    string                         `json:"target"`
	Root      string                         `json:"root"`
	Iterate   string                         `json:"iterate"`
	ChildRoot string                         `json:"child_root,omitempty"`
	Fields    map[string]MonitorFieldMapping `json:"fields"`
	Tiers     *MonitorTierMapping            `json:"tiers,omitempty"`
	Billing   *MonitorBillingMapping         `json:"billing,omitempty"`
}

// Billing rules classify the current model row in the browser. They neither
// filter rows nor read other responses, parent rows or local pricing hints.
type MonitorBillingMapping struct {
	Rules []MonitorBillingRule `json:"rules"`
}

type MonitorBillingRule struct {
	Match        string                    `json:"match"`
	Conditions   []MonitorBillingCondition `json:"conditions"`
	Unit         string                    `json:"unit"`
	RequestPrice *MonitorBillingPrice      `json:"request_price,omitempty"`
	Notes        string                    `json:"notes,omitempty"`
}

type MonitorBillingCondition struct {
	Path  string   `json:"path"`
	Op    string   `json:"op"`
	Value *float64 `json:"value,omitempty"`
}

type MonitorBillingPrice struct {
	Kind  string   `json:"kind"`
	Path  string   `json:"path"`
	Scale *float64 `json:"scale,omitempty"`
}

// Optional billing properties must be omitted rather than null. Keep this check
// at the mapping boundary so API and persisted extraction JSON agree, including
// a null billing object that a pointer's UnmarshalJSON would never receive.
func (mapping *MonitorMapping) UnmarshalJSON(data []byte) error {
	type mappingJSON MonitorMapping
	decoded := struct {
		*mappingJSON
		Billing common.RawMessage `json:"billing"`
	}{mappingJSON: &mappingJSON{}}
	if err := common.DecodeJsonBodyStrict(bytes.NewReader(data), &decoded); err != nil {
		return err
	}
	if len(decoded.Billing) != 0 {
		var value any
		if err := common.Unmarshal(decoded.Billing, &value); err != nil {
			return err
		}
		if monitorBillingContainsNull(value) {
			return errors.New("Invalid monitor billing rules")
		}
		if err := common.DecodeJsonBodyStrict(bytes.NewReader(decoded.Billing), &decoded.mappingJSON.Billing); err != nil {
			return err
		}
	}
	*mapping = MonitorMapping(*decoded.mappingJSON)
	return nil
}

func monitorBillingContainsNull(value any) bool {
	switch value := value.(type) {
	case nil:
		return true
	case map[string]any:
		for _, child := range value {
			if monitorBillingContainsNull(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if monitorBillingContainsNull(child) {
				return true
			}
		}
	}
	return false
}

type MonitorTierMapping struct {
	Path   string                         `json:"path"`
	Mode   string                         `json:"mode"`
	Basis  string                         `json:"basis"`
	Fields map[string]MonitorFieldMapping `json:"fields"`
}

type MonitorFieldMapping struct {
	Kind         string               `json:"kind"`
	Path         string               `json:"path,omitempty"`
	FallbackPath string               `json:"fallback_path,omitempty"`
	Source       string               `json:"source,omitempty"`
	Value        common.RawMessage    `json:"value,omitempty"`
	Multiply     *MonitorFieldMapping `json:"multiply,omitempty"`
	Divide       *MonitorFieldMapping `json:"divide,omitempty"`
	Scale        *float64             `json:"scale,omitempty"`
	Translate    map[string]string    `json:"translate,omitempty"`
}

type MonitorResultSource struct {
	RequestID  string `json:"request_id"`
	URL        string `json:"url"`
	Method     string `json:"method"`
	PageURL    string `json:"page_url"`
	Group      string `json:"group"`
	CapturedAt int64  `json:"captured_at"`
}

type MonitorBatchResult struct {
	SchemaVersion int                   `json:"schema_version"`
	FlowRevision  int64                 `json:"flow_revision"`
	Data          MonitorData           `json:"data"`
	Sources       []MonitorResultSource `json:"sources"`
	Complete      bool                  `json:"complete"`
	Missing       []string              `json:"missing"`
}

type MonitorResultRequest struct {
	LeaseToken string             `json:"lease_token"`
	Result     MonitorBatchResult `json:"result"`
}

type MonitorBinding struct {
	ID            string `json:"id"`
	ChannelID     int    `json:"channel_id"`
	AccountID     string `json:"account_id"`
	UpstreamGroup string `json:"upstream_group"`
	LocalModel    string `json:"local_model"` // empty uses the channel's mapping
	UpstreamModel string `json:"upstream_model"`
}

type MonitorScenario struct {
	Currency      string  `json:"currency"`
	USDExchange   float64 `json:"usd_exchange"`
	InputTokens   int     `json:"input_tokens"` // uncached text tokens
	OutputTokens  int     `json:"output_tokens"`
	CacheRead     int     `json:"cache_read"`
	CacheWrite    int     `json:"cache_write"`
	Requests      int     `json:"requests"`
	RevenueFactor float64 `json:"revenue_factor"` // actual receipts / issued face value
	FeeRate       float64 `json:"fee_rate"`
	FeeBearer     string  `json:"fee_bearer"`
	UserGroup     string  `json:"user_group"` // optional special group price
}

type MonitorBalance struct {
	Amount   *float64 `json:"amount"`
	Currency string   `json:"currency"`
	Kind     string   `json:"kind"`
}

type MonitorGroup struct {
	Name            string   `json:"name"` // stable group key used by model and channel bindings
	DisplayName     string   `json:"display_name,omitempty"`
	Ratio           *float64 `json:"ratio"`
	CostUnsupported string   `json:"cost_unsupported,omitempty"`
}

type MonitorModelPrice struct {
	Name              string              `json:"name"`
	Group             string              `json:"group"`
	Currency          string              `json:"currency"`
	BillingUnit       string              `json:"billing_unit"` // token, request, expression, unsupported
	TokenUnit         *float64            `json:"token_unit"`   // e.g. 1e6
	Input             *float64            `json:"input"`
	Output            *float64            `json:"output"`
	CacheRead         *float64            `json:"cache_read"`
	CacheWrite        *float64            `json:"cache_write"`
	RequestPrice      *float64            `json:"request_price"`
	IncludesGroup     *bool               `json:"includes_group"`
	Notes             string              `json:"notes"`
	Tiers             []MonitorPriceTier  `json:"tiers,omitempty"`
	TierMode          string              `json:"tier_mode,omitempty"`
	TierBasis         string              `json:"tier_basis,omitempty"`
	BillingExpression string              `json:"billing_expression,omitempty"`
	PricingPending    bool                `json:"pricing_pending,omitempty"`
	PriceSource       *MonitorPriceSource `json:"price_source,omitempty"`
}

// Provenance is derived only during server-side comparison. Browser uploads and
// stored captured prices must omit price_source entirely, including null.
func (price *MonitorModelPrice) UnmarshalJSON(data []byte) error {
	type priceJSON MonitorModelPrice
	decoded := struct {
		*priceJSON
		PriceSource common.RawMessage `json:"price_source"`
	}{priceJSON: &priceJSON{}}
	if err := common.DecodeJsonBodyStrict(bytes.NewReader(data), &decoded); err != nil {
		return err
	}
	if len(decoded.PriceSource) != 0 {
		return errors.New("Monitor snapshots must contain captured prices only")
	}
	*price = MonitorModelPrice(*decoded.priceJSON)
	return nil
}

type MonitorPriceTier struct {
	InputThreshold *int64   `json:"input_threshold"`
	Input          *float64 `json:"input"`
	Output         *float64 `json:"output"`
	CacheRead      *float64 `json:"cache_read"`
	CacheWrite     *float64 `json:"cache_write"`
}

type MonitorTopup struct {
	Reference string   `json:"reference"`
	Paid      *float64 `json:"paid"`
	Credited  *float64 `json:"credited"`
	Currency  string   `json:"currency"`
	Fee       *float64 `json:"fee"`
	FeeBearer string   `json:"fee_bearer"`
}

type MonitorData struct {
	SchemaVersion int                   `json:"schema_version,omitempty"`
	Sources       []MonitorResultSource `json:"sources,omitempty"`
	Missing       []string              `json:"missing,omitempty"`
	AccountID     string                `json:"account_id"` // upstream identity, not internal ID
	Balances      []MonitorBalance      `json:"balances"`
	Groups        []MonitorGroup        `json:"groups"`
	Models        []MonitorModelPrice   `json:"models"`
	Topups        []MonitorTopup        `json:"topups"`
	Warnings      []string              `json:"warnings"`
	PricingStatus string                `json:"pricing_status,omitempty"`
}

type MonitorJobRequest struct {
	AccountID string `json:"account_id"`
	DeviceID  string `json:"device_id"`
	FlowID    string `json:"flow_id"`
	Mode      string `json:"mode"` // collect, record
}

type MonitorFlowSaveRequest struct {
	LeaseToken string              `json:"lease_token"`
	Flow       MonitorFlow         `json:"flow"`
	Preview    *MonitorBatchResult `json:"preview,omitempty"`
}

type MonitorJobEvent struct {
	LeaseToken string        `json:"lease_token"`
	Type       string        `json:"type"` // progress, recording, complete, failed, awaiting_user
	Step       int           `json:"step"`
	Message    string        `json:"message"`
	Steps      []MonitorStep `json:"steps"`
	Complete   bool          `json:"complete"`
}

type MonitorExecution struct {
	ID         string         `json:"id"`
	LeaseToken string         `json:"lease_token"`
	Mode       string         `json:"mode"`
	Site       MonitorSite    `json:"site"`
	Account    MonitorAccount `json:"account"`
	Flow       MonitorFlow    `json:"flow"`
}

type MonitorProfitRow struct {
	BindingID     string                    `json:"binding_id,omitempty"`
	LocalGroup    string                    `json:"local_group"`
	LocalRatio    float64                   `json:"local_ratio"`
	ChannelID     int                       `json:"channel_id"`
	ChannelName   string                    `json:"channel_name"`
	AccountID     string                    `json:"account_id"`
	SiteName      string                    `json:"site_name"`
	UpstreamGroup string                    `json:"upstream_group"`
	UpstreamRatio *float64                  `json:"upstream_ratio"`
	LocalModel    string                    `json:"local_model"`
	UpstreamModel string                    `json:"upstream_model"`
	LocalTier     string                    `json:"local_tier,omitempty"`
	UpstreamTier  string                    `json:"upstream_tier,omitempty"`
	Revenue       *float64                  `json:"revenue"`
	Cost          *float64                  `json:"cost"`
	Profit        *float64                  `json:"profit"`
	Margin        *float64                  `json:"margin"`
	Status        string                    `json:"status"`
	Reason        string                    `json:"reason"`
	SnapshotID    string                    `json:"snapshot_id"`
	CapturedAt    int64                     `json:"captured_at"`
	TierSamples   []MonitorProfitTierSample `json:"tier_samples,omitempty"`
	Estimated     bool                      `json:"estimated"`
	PriceSource   *MonitorPriceSource       `json:"price_source,omitempty"`
}
