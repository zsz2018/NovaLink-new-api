package service

import (
	"errors"
	"math"
	"net/url"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

var monitorMappingFields = map[string][]string{
	"account_id": {"value"},
	"balances":   {"amount", "currency", "kind"},
	"groups":     {"name", "display_name", "ratio"},
	"models":     {"name", "group", "currency", "billing_unit", "token_unit", "input", "output", "cache_read", "cache_write", "request_price", "includes_group", "notes", "billing_expression"},
	"topups":     {"reference", "paid", "credited", "currency", "fee", "fee_bearer"},
}

func validMonitorPointer(path string) bool {
	if len(path) > 512 || strings.Contains(path, "{key}") || path != "" && !strings.HasPrefix(path, "/") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "__proto__" || part == "constructor" || part == "prototype" {
			return false
		}
		for i := 0; i < len(part); i++ {
			if part[i] == '~' {
				if i+1 >= len(part) || part[i+1] != '0' && part[i+1] != '1' {
					return false
				}
				i++
			}
		}
	}
	return true
}

func validMonitorNestedPointer(path string) bool {
	if path == "" || strings.Count(path, "/") > 40 || !validMonitorPointer(path) {
		return false
	}
	unescape := strings.NewReplacer("~1", "/", "~0", "~")
	for part := range strings.SplitSeq(path[1:], "/") {
		if monitorSensitiveKey(unescape.Replace(part)) {
			return false
		}
	}
	return true
}

func validMonitorTranslation(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= limit && strings.IndexFunc(value, unicode.IsControl) < 0 && !strings.Contains(strings.ToLower(value), "redacted")
}

func validateMonitorField(field MonitorFieldMapping, requests map[string]MonitorRequestRule, depth int, allowParent, allowFallback bool) error {
	if depth > 2 || !validMonitorPointer(field.Path) || field.Source != "" && requests[field.Source].ID == "" {
		return errors.New("Invalid monitor field mapping")
	}
	if field.Translate != nil {
		if len(field.Translate) == 0 || len(field.Translate) > 30 {
			return errors.New("Invalid monitor field mapping")
		}
		for source, output := range field.Translate {
			if !validMonitorTranslation(source, 80) || !validMonitorTranslation(output, 128) || slices.Contains([]string{"__proto__", "constructor", "prototype"}, source) {
				return errors.New("Invalid monitor field mapping")
			}
		}
	}
	if field.FallbackPath != "" {
		simple := field.Source == "" && len(field.Value) == 0 && field.Multiply == nil && field.Divide == nil && field.Scale == nil
		if !allowFallback || !simple || !validMonitorNestedPointer(field.Path) || !validMonitorNestedPointer(field.FallbackPath) {
			return errors.New("Invalid monitor field mapping")
		}
	}
	switch field.Kind {
	case "path":
		if len(field.Value) != 0 {
			return errors.New("Invalid monitor field mapping")
		}
	case "parent":
		if !allowParent || !validMonitorNestedPointer(field.Path) || field.Source != "" || len(field.Value) != 0 {
			return errors.New("Invalid monitor field mapping")
		}
	case "constant":
		var scalar any
		if field.Path != "" || field.Source != "" || len(field.Value) == 0 || len(field.Value) > 1024 || common.Unmarshal(field.Value, &scalar) != nil {
			return errors.New("Invalid monitor field mapping")
		}
		switch value := scalar.(type) {
		case nil, string, bool:
		case float64:
			if !monitorFinite(value) || value > 1e15 {
				return errors.New("Invalid monitor field mapping")
			}
		default:
			return errors.New("Invalid monitor field mapping")
		}
	case "key", "group":
		if field.Path != "" || field.Source != "" || len(field.Value) != 0 {
			return errors.New("Invalid monitor field mapping")
		}
	default:
		return errors.New("Invalid monitor field mapping")
	}
	if field.Scale != nil && (!monitorFinite(*field.Scale) || *field.Scale == 0 || *field.Scale > 1e15) {
		return errors.New("Invalid monitor field mapping")
	}
	for _, operand := range []*MonitorFieldMapping{field.Multiply, field.Divide} {
		if operand != nil {
			if err := validateMonitorField(*operand, requests, depth+1, allowParent, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMonitorBillingMapping(mapping MonitorMapping) error {
	if mapping.Billing == nil {
		return nil
	}
	if mapping.Target != "models" || len(mapping.Billing.Rules) < 1 || len(mapping.Billing.Rules) > 8 {
		return errors.New("Invalid monitor billing rules")
	}
	for _, field := range []string{"billing_unit", "request_price", "billing_expression"} {
		if _, exists := mapping.Fields[field]; exists {
			return errors.New("Invalid monitor billing rules")
		}
	}
	for _, rule := range mapping.Billing.Rules {
		if !slices.Contains([]string{"all", "any"}, rule.Match) || len(rule.Conditions) < 1 || len(rule.Conditions) > 8 || !slices.Contains([]string{"token", "request", "unsupported"}, rule.Unit) {
			return errors.New("Invalid monitor billing rules")
		}
		if !utf8.ValidString(rule.Notes) || len(rule.Notes) > 512 || strings.IndexFunc(rule.Notes, unicode.IsControl) >= 0 || strings.Contains(strings.ToLower(rule.Notes), "redacted") || rule.Unit == "unsupported" && strings.TrimSpace(rule.Notes) == "" {
			return errors.New("Invalid monitor billing rules")
		}
		for _, condition := range rule.Conditions {
			if !utf8.ValidString(condition.Path) || strings.IndexFunc(condition.Path, unicode.IsControl) >= 0 || !validMonitorNestedPointer(condition.Path) {
				return errors.New("Invalid monitor billing rules")
			}
			switch condition.Op {
			case "exists", "missing":
				if condition.Value != nil {
					return errors.New("Invalid monitor billing rules")
				}
			case "gt", "gte", "eq", "lt", "lte":
				if condition.Value == nil || !monitorFinite(*condition.Value) || *condition.Value > 1e15 {
					return errors.New("Invalid monitor billing rules")
				}
			default:
				return errors.New("Invalid monitor billing rules")
			}
		}
		if rule.Unit == "request" {
			price := rule.RequestPrice
			if price == nil || price.Kind != "path" || !utf8.ValidString(price.Path) || strings.IndexFunc(price.Path, unicode.IsControl) >= 0 || !validMonitorNestedPointer(price.Path) || price.Scale != nil && (!monitorFinite(*price.Scale) || *price.Scale == 0 || *price.Scale > 1e15) {
				return errors.New("Invalid monitor billing rules")
			}
		} else if rule.RequestPrice != nil {
			return errors.New("Invalid monitor billing rules")
		}
	}
	return nil
}

// Page URLs are browser navigation targets. Preserve safe view parameters, but
// never persist login callbacks or credential-bearing query strings.
func monitorCollectionPage(raw string, site MonitorSite) (string, error) {
	origin, err := monitorOrigin(raw)
	parsed, parseErr := url.Parse(raw)
	if err != nil || parseErr != nil || len(raw) > 2048 || origin != site.Origin && !slices.Contains(site.AllowedOrigins, origin) || parsed.Fragment != "" {
		return "", errors.New("Invalid monitor collection page")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) > 30 {
		return "", errors.New("Invalid monitor collection page")
	}
	for key, values := range query {
		if monitorSensitiveKey(key) || slices.Contains([]string{"code", "state", "auth", "authkey", "auth_key", "key", "jwt", "ticket", "signature", "session_token"}, strings.ToLower(key)) {
			return "", errors.New("Invalid monitor collection page")
		}
		for _, value := range values {
			if len(value) > 1024 || strings.Contains(strings.ToLower(value), "redacted") {
				return "", errors.New("Invalid monitor collection page")
			}
		}
	}
	parsed.Scheme, parsed.Host = strings.ToLower(parsed.Scheme), strings.ToLower(parsed.Host)
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func validateMonitorExtraction(extraction *MonitorExtraction, site MonitorSite) error {
	return validateMonitorExtractionSchema(extraction, site, false)
}

// ValidateMonitorExtractionDraft validates proposed mappings before all required
// collection targets have been configured. Every supplied rule remains strict.
func ValidateMonitorExtractionDraft(extraction *MonitorExtraction, site MonitorSite) error {
	return validateMonitorExtractionSchema(extraction, site, true)
}

func validateMonitorExtractionSchema(extraction *MonitorExtraction, site MonitorSite, draft bool) error {
	if extraction == nil {
		return errors.New("Invalid monitor extraction rules")
	}
	if extraction.Adapter != "" && extraction.Adapter != "newapi" && extraction.Adapter != "sub2api" {
		return errors.New("Invalid monitor extraction rules")
	}
	if extraction.Version != 1 || !draft && len(extraction.Requests) < 1 || len(extraction.Requests) > 100 || len(extraction.Mappings) > 200 {
		return errors.New("Invalid monitor extraction rules")
	}
	if extraction.Adapter != "" && (extraction.Mappings == nil || len(extraction.Mappings) != 0) {
		return errors.New("Invalid monitor extraction rules")
	}
	requests := map[string]MonitorRequestRule{}
	pages := map[string]bool{}
	for _, request := range extraction.Requests {
		origin, err := monitorOrigin(request.URL)
		parsed, parseErr := url.Parse(request.URL)
		if err != nil || parseErr != nil || origin != site.Origin && !slices.Contains(site.AllowedOrigins, origin) || parsed.RawQuery != "" || parsed.Fragment != "" || len(request.URL) > 2048 || request.ID == "" || len(request.ID) > 64 || requests[request.ID].ID != "" || !slices.Contains([]string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"}, request.Method) || len(request.Query) > 30 {
			return errors.New("Invalid monitor extraction rules")
		}
		for key, value := range request.Query {
			if key == "" || len(key) > 128 || len(value) > 1024 || monitorSensitiveKey(key) {
				return errors.New("Invalid monitor extraction rules")
			}
		}
		if len(request.PageURLs) > 20 {
			return errors.New("Invalid monitor collection page")
		}
		requestPages := map[string]bool{}
		for _, raw := range request.PageURLs {
			page, err := monitorCollectionPage(raw, site)
			if err != nil || requestPages[page] {
				return errors.New("Invalid monitor collection page")
			}
			requestPages[page], pages[page] = true, true
		}
		requests[request.ID] = request
	}
	if len(pages) > 100 {
		return errors.New("Invalid monitor collection page")
	}
	if extraction.Adapter != "" {
		return nil
	}
	seen := map[string]bool{}
	identity := false
	targets := map[string]bool{}
	for _, mapping := range extraction.Mappings {
		allowed := monitorMappingFields[mapping.Target]
		if mapping.ID == "" || len(mapping.ID) > 64 || seen[mapping.ID] || requests[mapping.RequestID].ID == "" || len(allowed) == 0 || !validMonitorPointer(mapping.Root) || !slices.Contains([]string{"single", "array", "entries", "nested_array"}, mapping.Iterate) || len(mapping.Fields) == 0 || len(mapping.Fields) > len(allowed) {
			return errors.New("Invalid monitor extraction rules")
		}
		nested := mapping.Iterate == "nested_array"
		if nested {
			if mapping.Target != "models" || !validMonitorNestedPointer(mapping.ChildRoot) {
				return errors.New("Invalid monitor extraction rules")
			}
		} else if mapping.ChildRoot != "" {
			return errors.New("Invalid monitor extraction rules")
		}
		if err := validateMonitorBillingMapping(mapping); err != nil {
			return err
		}
		if err := validateMonitorTierMapping(mapping, requests); err != nil {
			return err
		}
		seen[mapping.ID] = true
		targets[mapping.Target] = true
		for name, field := range mapping.Fields {
			if !slices.Contains(allowed, name) {
				return errors.New("Invalid monitor field mapping")
			}
			if name == "billing_expression" && (field.Kind != "path" || !validMonitorNestedPointer(field.Path) || field.FallbackPath != "" || field.Translate != nil || field.Multiply != nil || field.Divide != nil || field.Scale != nil) {
				return errors.New("Invalid upstream billing expression")
			}
			allowFallback := mapping.Target == "groups" && name == "name" && field.Kind == "path" || mapping.Target == "models" && name == "group" && nested && field.Kind == "parent"
			if err := validateMonitorField(field, requests, 0, nested, allowFallback); err != nil {
				return err
			}
		}
		if mapping.Target == "account_id" {
			field, exists := mapping.Fields["value"]
			if !exists || field.Kind != "path" || field.Multiply != nil || field.Divide != nil || field.Scale != nil || mapping.Iterate != "single" {
				return errors.New("Account identity must map to a response field")
			}
			identity = true
		}
	}
	if !draft && (!identity || !targets["balances"] || !targets["groups"]) {
		return errors.New("Map account identity, balances and groups before collecting")
	}
	return nil
}

// A paired, root-owned browser is the authenticated source of these selected
// fields. The server validates the frozen recipe and business schema, without
// receiving or reconstructing upstream response bodies.
func validateMonitorBatch(result MonitorBatchResult, execution MonitorExecution, since int64) (MonitorData, error) {
	data := result.Data
	if result.SchemaVersion != 1 || result.FlowRevision != execution.Flow.Revision || execution.Flow.Extraction == nil {
		return data, errors.New("Monitor result does not match the saved mapping revision")
	}
	if err := validateMonitorExtraction(execution.Flow.Extraction, execution.Site); err != nil {
		return data, err
	}
	if data.AccountID == "" || len(data.AccountID) > 255 || execution.Account.ExternalID != "" && data.AccountID != execution.Account.ExternalID {
		return data, errors.New("Captured account identity does not match the configured account")
	}
	if len(result.Sources) == 0 || len(result.Sources) > 300 || len(data.Balances) > 100 || len(data.Groups) > 500 || len(data.Models) > 20000 || len(data.Topups) > 5000 || len(data.Warnings) > 100 || len(result.Missing) > 100 {
		return data, errors.New("Invalid monitor structured result")
	}
	requests := map[string]MonitorRequestRule{}
	for _, request := range execution.Flow.Extraction.Requests {
		requests[request.ID] = request
	}
	seen := map[string]bool{}
	seenPages := map[string]bool{}
	for _, source := range result.Sources {
		request, exists := requests[source.RequestID]
		if !exists || source.Method != request.Method || source.URL != request.URL || len(source.Group) > 128 || source.CapturedAt < since-300 || source.CapturedAt <= 0 || source.CapturedAt > common.GetTimestamp()+300 {
			return data, errors.New("Monitor result source does not match the configured requests")
		}
		page, err := monitorCollectionPage(source.PageURL, execution.Site)
		if err != nil {
			return data, errors.New("Monitor result source does not match the configured requests")
		}
		if len(request.PageURLs) > 0 {
			matched := false
			for _, raw := range request.PageURLs {
				expected, err := monitorCollectionPage(raw, execution.Site)
				if err == nil && expected == page {
					matched = true
					break
				}
			}
			if !matched {
				return data, errors.New("Monitor result source does not match the configured requests")
			}
		}
		seen[source.RequestID] = true
		seenPages[source.RequestID+"\x00"+page] = true
	}
	for _, list := range [][]string{data.Warnings, result.Missing} {
		for _, message := range list {
			if len(message) > 512 {
				return data, errors.New("Invalid monitor structured result")
			}
		}
	}
	if data.PricingStatus != "" && !slices.Contains([]string{"available", "partial", "unavailable"}, data.PricingStatus) {
		return data, errors.New("Invalid monitor structured result")
	}
	missing := len(data.Balances) == 0 || len(data.Groups) == 0 || len(result.Missing) != 0 || len(seen) != len(requests)
	for _, rule := range requests {
		for _, raw := range rule.PageURLs {
			page, _ := monitorCollectionPage(raw, execution.Site)
			if !seenPages[rule.ID+"\x00"+page] {
				missing = true
			}
		}
	}
	balanceKeys := map[string]bool{}
	for i := range data.Balances {
		balance := &data.Balances[i]
		key := balance.Currency + "\x00" + balance.Kind
		if balance.Currency != execution.Site.Currency || len(balance.Kind) > 64 || balanceKeys[key] {
			return data, errors.New("Invalid monitor structured result")
		}
		if balance.Amount != nil && (math.IsNaN(*balance.Amount) || math.IsInf(*balance.Amount, 0) || math.Abs(*balance.Amount) > 1e15) {
			return data, errors.New("Invalid monitor structured result")
		}
		balanceKeys[key] = true
		missing = missing || balance.Amount == nil
	}
	groups := map[string]bool{}
	for i := range data.Groups {
		group := &data.Groups[i]
		if group.Name == "" || len(group.Name) > 128 || groups[group.Name] {
			return data, errors.New("Invalid or duplicate upstream group")
		}
		if len(group.DisplayName) > 512 || strings.IndexFunc(group.DisplayName, unicode.IsControl) >= 0 || strings.Contains(strings.ToLower(group.DisplayName), "[redacted]") {
			return data, errors.New("Invalid monitor structured result")
		}
		if group.CostUnsupported != "" && (!utf8.ValidString(group.CostUnsupported) || len(group.CostUnsupported) > 512 || strings.TrimSpace(group.CostUnsupported) == "" || strings.IndexFunc(group.CostUnsupported, unicode.IsControl) >= 0 || strings.Contains(strings.ToLower(group.CostUnsupported), "[redacted]")) {
			return data, errors.New("Invalid monitor structured result")
		}
		group.DisplayName = strings.TrimSpace(group.DisplayName)
		groups[group.Name] = true
		missing = missing || group.Ratio == nil
	}
	modelKeys := map[string]bool{}
	checkedExpressions := map[string]bool{}
	for i := range data.Models {
		price := &data.Models[i]
		key := price.Group + "\x00" + price.Name
		if price.PriceSource != nil {
			return data, errors.New("Monitor snapshots must contain captured prices only")
		}
		if price.Name == "" || len(price.Name) > 255 || len(price.Notes) > 512 || modelKeys[key] || !groups[price.Group] || price.Currency != execution.Site.Currency || !slices.Contains([]string{"", "token", "request", "expression", "unsupported"}, price.BillingUnit) {
			return data, errors.New("Invalid model identity, group or currency")
		}
		modelKeys[key] = true
		if err := validateMonitorPriceTiers(*price); err != nil {
			return data, err
		}
		if price.BillingExpression != "" {
			if price.BillingUnit != "expression" {
				return data, errors.New("Invalid upstream billing expression")
			}
			if !checkedExpressions[price.BillingExpression] {
				if err := validateMonitorPriceExpression(price.BillingExpression); err != nil {
					return data, err
				}
				checkedExpressions[price.BillingExpression] = true
			}
		}
		if price.TokenUnit != nil && (*price.TokenUnit <= 0 || *price.TokenUnit > 1e12) {
			return data, errors.New("Token unit must be positive")
		}
	}
	data.PricingStatus = monitorCapturedPricingStatus(data.Models)
	topupKeys := map[string]bool{}
	for i := range data.Topups {
		topup := &data.Topups[i]
		if topup.Reference == "" || len(topup.Reference) > 255 || topupKeys[topup.Reference] || topup.Currency != execution.Site.Currency || !slices.Contains([]string{"self", "other", "unknown"}, topup.FeeBearer) {
			return data, errors.New("Invalid monitor structured result")
		}
		topupKeys[topup.Reference] = true
	}
	var numbers []*float64
	for _, group := range data.Groups {
		numbers = append(numbers, group.Ratio)
	}
	for _, price := range data.Models {
		numbers = append(numbers, price.TokenUnit, price.Input, price.Output, price.CacheRead, price.CacheWrite, price.RequestPrice)
	}
	for _, topup := range data.Topups {
		numbers = append(numbers, topup.Paid, topup.Credited, topup.Fee)
	}
	for _, number := range numbers {
		if number != nil && (!monitorFinite(*number) || *number > 1e15) {
			return data, errors.New("Monitor values must be finite and non-negative")
		}
	}
	if result.Complete && missing {
		return data, errors.New("Monitor result is missing required fields or responses")
	}
	data.SchemaVersion, data.Sources, data.Missing = 1, result.Sources, result.Missing
	encoded, err := common.Marshal(data)
	if err != nil || len(encoded) > 8<<20 {
		return data, errors.New("Invalid monitor structured result")
	}
	if execution.Account.Password != "" && strings.Contains(string(encoded), execution.Account.Password) {
		return data, errors.New("Monitor structured results cannot contain credentials")
	}
	return data, nil
}

func SubmitMonitorBrowserResult(device *model.MonitorDevice, id string, request MonitorResultRequest) (map[string]any, error) {
	now := common.GetTimestamp()
	var job model.MonitorJob
	if request.LeaseToken == "" || model.DB.Where("id = ? AND device_id = ? AND mode = ? AND status IN ? AND lease_hash = ? AND lease_until >= ?", id, device.ID, "collect", []string{"running", "awaiting_user"}, monitorHash(request.LeaseToken), now).First(&job).Error != nil {
		return nil, errors.New("Invalid or expired monitor execution lease")
	}
	execution, err := monitorExecution(&job)
	if err != nil {
		return nil, err
	}
	data, err := validateMonitorBatch(request.Result, execution, job.CreatedAt)
	if err != nil {
		return nil, err
	}
	encoded, err := common.Marshal(data)
	if err != nil {
		return nil, err
	}
	snapshot := model.MonitorSnapshot{ID: monitorID("snap_"), JobID: id, AccountID: job.AccountID, Data: model.LongText(encoded), Complete: request.Result.Complete, Confirmed: request.Result.Complete, CreatedAt: now}
	if snapshot.Confirmed {
		snapshot.ConfirmedAt = now
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := monitorBrowserOwner(tx, device.ID, now); err != nil {
			return err
		}
		updated := tx.Model(&model.MonitorJob{}).Where("id = ? AND device_id = ? AND mode = ? AND status IN ? AND lease_hash = ? AND lease_until >= ? AND flow_revision = ?", id, device.ID, "collect", []string{"running", "awaiting_user"}, monitorHash(request.LeaseToken), common.GetTimestamp(), request.Result.FlowRevision).Updates(map[string]any{"status": "completed", "complete": snapshot.Complete, "lease_hash": "", "control": "", "message": "", "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("Monitor execution lease changed")
		}
		return tx.Create(&snapshot).Error
	})
	if err != nil {
		return nil, err
	}
	return monitorSnapshotResponse(snapshot), nil
}

func monitorBrowserOwner(tx *gorm.DB, deviceID string, now int64) error {
	var device model.MonitorDevice
	if err := tx.Where("id = ? AND revoked_at = 0 AND expires_at > ?", deviceID, now).First(&device).Error; err != nil {
		return errors.New("Invalid device token")
	}
	var owner model.User
	if err := tx.Select("id", "role", "status").First(&owner, device.OwnerID).Error; err != nil || owner.Role != common.RoleRootUser || owner.Status != common.UserStatusEnabled {
		return errors.New("Monitor device owner is no longer authorized")
	}
	return nil
}
