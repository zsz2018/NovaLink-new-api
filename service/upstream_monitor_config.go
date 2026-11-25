package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"maps"
	"math"
	"net/url"
	"os"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

var ErrMonitorConflict = errors.New("Monitor configuration changed; reload before saving")

const monitorMaxConfigBytes = 2 << 20

func monitorID(prefix string) string {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(value[:])
}

func monitorHash(value string) string {
	result := sha256.Sum256([]byte(value))
	return hex.EncodeToString(result[:])
}

func MonitorKeyReady() bool { _, err := monitorCipher(); return err == nil }

func monitorCipher() (cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(os.Getenv("UPSTREAM_MONITOR_KEY"))
	if err != nil || len(key) != 32 {
		return nil, errors.New("UPSTREAM_MONITOR_KEY must be a base64-encoded 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func monitorEncrypt(value string) (string, error) {
	aead, err := monitorCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return "v1:" + base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(value), []byte("novalink-monitor-v1"))), nil
}

func monitorDecrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	aead, err := monitorCipher()
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(value, "v1:") {
		return "", errors.New("Invalid monitor secret version")
	}
	encoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "v1:"))
	if err != nil || len(encoded) < aead.NonceSize() {
		return "", errors.New("Invalid monitor secret")
	}
	decoded, err := aead.Open(nil, encoded[:aead.NonceSize()], encoded[aead.NonceSize():], []byte("novalink-monitor-v1"))
	if err != nil {
		return "", errors.New("Unable to decrypt monitor secret")
	}
	return string(decoded), nil
}

func monitorDefaultConfig() MonitorConfig {
	return MonitorConfig{Sites: []MonitorSite{}, Accounts: []MonitorAccount{}, Flows: []MonitorFlow{}, Bindings: []MonitorBinding{}, Scenario: MonitorScenario{Currency: "CNY", USDExchange: 7, InputTokens: 1000, OutputTokens: 1000, Requests: 1, RevenueFactor: 1, FeeBearer: "unknown"}}
}

func GetMonitorConfig() (MonitorConfig, error) {
	config := monitorDefaultConfig()
	var stored model.MonitorConfiguration
	err := model.DB.First(&stored, "id = ?", 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	if err = common.UnmarshalJsonStr(string(stored.Content), &config); err != nil {
		return config, err
	}
	config.Version = stored.Version
	return config, nil
}

func monitorSecrets() (map[string]string, error) {
	secrets := map[string]string{}
	var stored model.MonitorConfiguration
	err := model.DB.First(&stored, "id = ?", 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return secrets, nil
	}
	if err != nil {
		return nil, err
	}
	if stored.Secrets == "" {
		return secrets, nil
	}
	decoded, err := monitorDecrypt(string(stored.Secrets))
	if err != nil {
		return nil, err
	}
	if err := common.UnmarshalJsonStr(decoded, &secrets); err != nil {
		return nil, err
	}
	return secrets, nil
}

func SaveMonitorConfig(config MonitorConfig) (MonitorConfig, error) {
	if err := validateMonitorConfig(config); err != nil {
		return config, err
	}
	secrets, err := monitorSecrets()
	if err != nil {
		return config, err
	}
	loginSecrets := slices.Collect(maps.Values(secrets))
	for _, account := range config.Accounts {
		if account.Password != "" {
			loginSecrets = append(loginSecrets, account.Password)
		}
	}
	for _, flow := range config.Flows {
		if err := validateMonitorFlowSecrets(flow, loginSecrets); err != nil {
			return config, err
		}
	}
	for _, account := range config.Accounts {
		if account.OfficialEstimate == nil {
			continue
		}
		for _, selection := range account.OfficialEstimate.Models {
			if err := validateMonitorSecretFields([]any{selection.Name, selection.Group, selection.EntryID, account.OfficialEstimate.CatalogVersion}, loginSecrets); err != nil {
				return config, err
			}
		}
	}
	keep := map[string]string{}
	for i := range config.Accounts {
		account := &config.Accounts[i]
		key := "password:" + account.ID
		if account.ClearPassword {
			delete(secrets, key)
		}
		if account.Password != "" {
			secrets[key] = account.Password
		}
		if secrets[key] != "" {
			keep[key] = secrets[key]
		}
		account.HasPassword = secrets[key] != ""
		account.Password, account.ClearPassword = "", false
	}
	secretText := ""
	if len(keep) > 0 {
		encoded, err := common.Marshal(keep)
		if err != nil {
			return config, err
		}
		secretText, err = monitorEncrypt(string(encoded))
		if err != nil {
			return config, err
		}
	}
	expected := config.Version
	config.Version++
	encoded, err := common.Marshal(config)
	if err != nil {
		return config, err
	}
	if len(encoded) > monitorMaxConfigBytes {
		return config, errors.New("Monitor configuration is too large")
	}
	if expected == 0 {
		err = model.DB.Create(&model.MonitorConfiguration{ID: 1, Version: config.Version, Content: model.LongText(encoded), Secrets: model.LongText(secretText)}).Error
		if err != nil {
			return config, ErrMonitorConflict
		}
	} else {
		result := model.DB.Model(&model.MonitorConfiguration{}).Where("id = ? AND version = ?", 1, expected).Updates(map[string]any{"version": config.Version, "content": string(encoded), "secrets": secretText})
		if result.Error != nil {
			return config, result.Error
		}
		if result.RowsAffected != 1 {
			return config, ErrMonitorConflict
		}
	}
	return config, nil
}

func monitorOrigin(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", errors.New("Invalid monitor URL")
	}
	if parsed.Scheme == "http" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "::1" {
		return "", errors.New("Monitor websites must use HTTPS")
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host), nil
}

func monitorFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func monitorFee(bearer string, rate float64) bool {
	return slices.Contains([]string{"self", "other", "unknown"}, bearer) && monitorFinite(rate) && rate < 1
}

func validateMonitorSelector(selector string) error {
	if strings.TrimSpace(selector) == "" || utf8.RuneCountInString(selector) > 2048 {
		return errors.New("Invalid monitor flow")
	}
	semantic, ok := strings.CutPrefix(selector, "semantic=")
	if !ok {
		return nil // CSS and XPath remain browser-validated selectors.
	}
	var properties map[string]common.RawMessage
	if err := common.UnmarshalJsonStr(semantic, &properties); err != nil || len(properties) == 0 || len(properties) > 5 {
		return errors.New("Invalid monitor selector")
	}
	values := make(map[string]string, len(properties))
	for key, raw := range properties {
		var value string
		if !slices.Contains([]string{"role", "name", "label", "placeholder", "scope"}, key) || common.GetJsonType(raw) != "string" || common.Unmarshal(raw, &value) != nil || utf8.RuneCountInString(value) > 512 {
			return errors.New("Invalid monitor selector")
		}
		if key == "scope" && strings.HasPrefix(strings.TrimSpace(value), "semantic=") {
			return errors.New("Invalid monitor selector")
		}
		values[key] = strings.TrimSpace(value)
	}
	if values["label"] == "" && values["placeholder"] == "" && (values["role"] == "" || values["name"] == "") {
		return errors.New("Invalid monitor selector")
	}
	return nil
}

func validateMonitorPageURL(raw string, site MonitorSite, exact bool) error {
	origin, err := monitorOrigin(raw)
	if err != nil || len(raw) > 2048 || strings.Contains(raw, "\\") || origin != site.Origin && !slices.Contains(site.AllowedOrigins, origin) || exact && strings.ContainsAny(raw, "?#") {
		return errors.New("Monitor navigation is outside the configured origins")
	}
	return nil
}

func validateMonitorCondition(condition *MonitorCondition, site MonitorSite) error {
	if condition == nil || utf8.RuneCountInString(condition.Text) > 512 {
		return errors.New("Invalid monitor condition")
	}
	if condition.URL != "" {
		if condition.Selector != "" || condition.State != "" || condition.Text != "" {
			return errors.New("Invalid monitor condition")
		}
		return validateMonitorPageURL(condition.URL, site, false)
	}
	if !slices.Contains([]string{"", "visible", "hidden", "enabled", "disabled"}, condition.State) {
		return errors.New("Invalid monitor condition")
	}
	return validateMonitorSelector(condition.Selector)
}

// The same validator handles published flows and recorded trees. Request IDs
// select extraction rules only; they cannot jump to or invoke another step.
type monitorStepValidation struct {
	site        MonitorSite
	requests    map[string]bool
	ids         map[string]bool
	count       int
	credentials int
	login       *MonitorLogin
}

func (validation *monitorStepValidation) steps(steps []MonitorStep, depth int) error {
	if len(steps) > 0 && depth > 4 {
		return errors.New("Invalid monitor step limits")
	}
	for _, step := range steps {
		validation.count++
		if validation.count > 500 {
			return errors.New("Too many monitor steps")
		}
		if step.TimeoutMS < 0 || step.TimeoutMS > 120000 || utf8.RuneCountInString(step.ID) > 128 || utf8.RuneCountInString(step.Name) > 128 || utf8.RuneCountInString(step.Value) > 8192 || utf8.RuneCountInString(step.Text) > 512 || utf8.RuneCountInString(step.Group) > 128 || utf8.RuneCountInString(step.RequestID) > 128 {
			return errors.New("Invalid monitor step limits")
		}
		if step.ID != "" {
			if validation.ids[step.ID] {
				return errors.New("Invalid monitor flow")
			}
			validation.ids[step.ID] = true
		}
		if step.Selector != "" {
			if err := validateMonitorSelector(step.Selector); err != nil {
				return err
			}
		}
		if step.State != "" && (!slices.Contains([]string{"visible", "hidden", "enabled", "disabled"}, step.State) || step.Action != "wait" && step.Action != "assert") {
			return errors.New("Invalid monitor condition")
		}
		if validation.login != nil && (step.Group != "" || step.Action == "repeat") {
			return errors.New("Invalid monitor flow")
		}
		if step.Action == "input" && step.Variable == "" && step.Value != "" && (monitorSensitiveKey(step.Selector) || strings.Contains(strings.ToLower(step.Selector), "otp")) {
			return errors.New("Sensitive input values cannot be recorded literally")
		}
		if step.Variable != "" {
			if validation.login == nil || step.Action != "input" || !slices.Contains([]string{"username", "password"}, step.Variable) {
				return errors.New("Invalid monitor variable")
			}
			if step.Value != "" {
				return errors.New("Credential variables cannot contain literal values")
			}
			validation.credentials++
		}
		if step.RequestID != "" && (step.Action != "wait_response" || !validation.requests[step.RequestID]) {
			return errors.New("Invalid monitor request reference")
		}
		if step.Action != "condition" && step.Action != "repeat" && (step.Condition != nil || step.Steps != nil || step.Otherwise != nil || step.MaxIterations != 0) {
			return errors.New("Invalid monitor flow")
		}
		dom := slices.Contains([]string{"click", "check", "focus", "input", "select", "key", "wait", "assert", "scroll"}, step.Action)
		if step.URL != "" {
			if step.Action != "navigate" && (!dom || validation.login == nil) {
				return errors.New("Invalid monitor flow")
			}
			if err := validateMonitorPageURL(step.URL, validation.site, step.Action != "navigate"); err != nil {
				return err
			}
		}
		switch step.Action {
		case "navigate":
			if step.URL == "" || step.Selector != "" || step.Text != "" || step.Value != "" {
				return errors.New("Invalid monitor flow")
			}
		case "click", "check", "focus", "assert":
			if strings.TrimSpace(step.Selector) == "" || step.Value != "" {
				return errors.New("Invalid monitor flow")
			}
		case "input":
			if strings.TrimSpace(step.Selector) == "" || validation.login != nil && step.Variable == "" {
				return errors.New("Invalid monitor variable")
			}
		case "select":
			if strings.TrimSpace(step.Selector) == "" {
				return errors.New("Invalid monitor flow")
			}
		case "key":
			if strings.TrimSpace(step.Selector) == "" {
				return errors.New("Invalid monitor flow")
			}
		case "wait":
			if step.Value != "" || step.Selector == "" && (step.State != "" || step.Text != "") {
				return errors.New("Invalid monitor flow")
			}
		case "scroll":
		case "checkpoint":
			if step.Selector != "" || step.Value != "" {
				return errors.New("Invalid monitor flow")
			}
		case "wait_response":
			if step.RequestID == "" || step.Selector != "" || step.Text != "" || step.Value != "" {
				return errors.New("Invalid monitor request reference")
			}
		case "condition", "repeat":
			if step.Selector != "" || step.Text != "" || step.Value != "" {
				return errors.New("Invalid monitor flow")
			}
			if err := validateMonitorCondition(step.Condition, validation.site); err != nil {
				return err
			}
			if step.Action == "condition" && (len(step.Steps)+len(step.Otherwise) == 0 || step.MaxIterations != 0) || step.Action == "repeat" && (len(step.Steps) == 0 || step.Otherwise != nil || step.MaxIterations < 1 || step.MaxIterations > 100) {
				return errors.New("Invalid monitor step limits")
			}
			if err := validation.steps(step.Steps, depth+1); err != nil {
				return err
			}
			if err := validation.steps(step.Otherwise, depth+1); err != nil {
				return err
			}
		default:
			return errors.New("Invalid monitor action")
		}
	}
	return nil
}

func validateMonitorFlowSteps(flow MonitorFlow, site MonitorSite) error {
	validation := monitorStepValidation{site: site, ids: map[string]bool{}, requests: map[string]bool{}}
	if flow.Extraction != nil {
		for _, request := range flow.Extraction.Requests {
			validation.requests[request.ID] = true
		}
	}
	if err := validation.steps(flow.Steps, 1); err != nil {
		return err
	}
	login := flow.Login
	if login == nil {
		return nil
	}
	if err := validateMonitorPageURL(login.URL, site, true); err != nil {
		return err
	}
	if len(login.Steps) == 0 || login.TimeoutMS != 0 && (login.TimeoutMS < 1000 || login.TimeoutMS > 120000) {
		return errors.New("Invalid monitor step limits")
	}
	if login.Success != nil {
		if err := validateMonitorCondition(login.Success, site); err != nil {
			return err
		}
	}
	validation.login = login
	before := validation.count
	if err := validation.steps(login.Steps, 1); err != nil {
		return err
	}
	if validation.count-before > 100 {
		return errors.New("Invalid monitor step limits")
	}
	if validation.credentials == 0 {
		return errors.New("Invalid monitor variable")
	}
	return nil
}

// Scan user-authored metadata recursively, including encoded URLs and mapping
// constants. Credential variable names and action enums are protocol values.
func validateMonitorFlowSecrets(flow MonitorFlow, secrets []string) error {
	if err := validateMonitorSecretFields([]any{flow.ID, flow.SiteID, flow.Name}, secrets); err != nil {
		return err
	}
	if err := validateMonitorStepSecrets(flow.Steps, secrets); err != nil {
		return err
	}
	if flow.Login != nil {
		if err := validateMonitorSecretFields(flow.Login.URL, secrets); err != nil {
			return err
		}
		if err := validateMonitorConditionSecrets(flow.Login.Success, secrets); err != nil {
			return err
		}
		if err := validateMonitorStepSecrets(flow.Login.Steps, secrets); err != nil {
			return err
		}
	}
	if flow.Extraction != nil {
		encoded, err := common.Marshal(flow.Extraction)
		if err != nil {
			return err
		}
		var extraction any
		if err := common.Unmarshal(encoded, &extraction); err != nil {
			return err
		}
		return validateMonitorSecretFields(extraction, secrets)
	}
	return nil
}

func validateMonitorStepSecrets(steps []MonitorStep, secrets []string) error {
	for _, step := range steps {
		if err := validateMonitorSecretFields([]any{step.ID, step.Name, step.URL, step.Selector, step.Text, step.Value, step.Group, step.RequestID}, secrets); err != nil {
			return err
		}
		if err := validateMonitorConditionSecrets(step.Condition, secrets); err != nil {
			return err
		}
		if err := validateMonitorStepSecrets(step.Steps, secrets); err != nil {
			return err
		}
		if err := validateMonitorStepSecrets(step.Otherwise, secrets); err != nil {
			return err
		}
	}
	return nil
}

func validateMonitorConditionSecrets(condition *MonitorCondition, secrets []string) error {
	if condition == nil {
		return nil
	}
	return validateMonitorSecretFields([]any{condition.Selector, condition.URL, condition.Text}, secrets)
}

func validateMonitorSecretFields(value any, secrets []string) error {
	switch value := value.(type) {
	case string:
		decoded, _ := url.PathUnescape(value)
		formDecoded, _ := url.QueryUnescape(value)
		for _, secret := range secrets {
			if secret != "" && (strings.Contains(value, secret) || strings.Contains(decoded, secret) || strings.Contains(formDecoded, secret)) {
				return errors.New("Recorded passwords must use a password variable")
			}
		}
		if semantic, ok := strings.CutPrefix(value, "semantic="); ok {
			var properties map[string]any
			if common.UnmarshalJsonStr(semantic, &properties) == nil {
				return validateMonitorSecretFields(properties, secrets)
			}
		}
	case []any:
		for _, child := range value {
			if err := validateMonitorSecretFields(child, secrets); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, child := range value {
			if err := validateMonitorSecretFields(key, secrets); err != nil {
				return err
			}
			if err := validateMonitorSecretFields(child, secrets); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMonitorConfig(config MonitorConfig) error {
	sites, err := validateMonitorConfigStructure(config)
	if err != nil {
		return err
	}
	for _, flow := range config.Flows {
		if err := validateMonitorExtraction(flow.Extraction, sites[flow.SiteID]); err != nil {
			return err
		}
	}
	return nil
}

// Browser recordings replace one flow's extraction rules. All configuration
// limits, page steps and resource relationships remain subject to the same
// checks as a full configuration save, including unrelated accounts and bindings.
func validateMonitorConfigStructure(config MonitorConfig) (map[string]MonitorSite, error) {
	if config.Version < 0 || len(config.Sites) > 100 || len(config.Accounts) > 500 || len(config.Flows) > 200 || len(config.Bindings) > 5000 {
		return nil, errors.New("Invalid monitor configuration limits")
	}
	sites := map[string]MonitorSite{}
	for _, site := range config.Sites {
		origin, err := monitorOrigin(site.Origin)
		if err != nil || origin != site.Origin || site.ID == "" || len(site.ID) > 64 || site.Name == "" || !monitorFinite(site.FXRate) || site.FXRate == 0 || site.Currency == "" {
			return nil, errors.New("Invalid monitor site")
		}
		if _, exists := sites[site.ID]; exists {
			return nil, errors.New("Duplicate monitor site")
		}
		for _, allowed := range site.AllowedOrigins {
			if normalized, err := monitorOrigin(allowed); err != nil || normalized != allowed {
				return nil, errors.New("Invalid allowed origin")
			}
		}
		sites[site.ID] = site
	}
	flows := map[string]MonitorFlow{}
	for _, flow := range config.Flows {
		site, exists := sites[flow.SiteID]
		if !exists || flow.ID == "" || len(flow.ID) > 64 || flow.Revision < 1 || flow.CaptureSeconds < 0 || flow.CaptureSeconds > 60 {
			return nil, errors.New("Invalid monitor flow")
		}
		if _, exists := flows[flow.ID]; exists {
			return nil, errors.New("Duplicate monitor flow")
		}
		if err := validateMonitorFlowSteps(flow, site); err != nil {
			return nil, err
		}
		flows[flow.ID] = flow
	}
	accounts := map[string]bool{}
	for _, account := range config.Accounts {
		_, exists := sites[account.SiteID]
		if !exists || account.ID == "" || len(account.ID) > 64 || len(account.Password) > 4096 || !monitorFinite(account.PurchaseFactor) || account.PurchaseFactor == 0 || !monitorFee(account.FeeBearer, account.FeeRate) || account.IntervalMinutes < 0 || account.IntervalMinutes > 525600 {
			return nil, errors.New("Invalid monitor account")
		}
		if accounts[account.ID] {
			return nil, errors.New("Duplicate monitor account")
		}
		if account.FlowID != "" {
			if flow, exists := flows[account.FlowID]; !exists || flow.SiteID != account.SiteID {
				return nil, errors.New("Monitor account flow does not match its site")
			}
		}
		if err := validateMonitorOfficialEstimate(account.OfficialEstimate, sites[account.SiteID]); err != nil {
			return nil, err
		}
		accounts[account.ID] = true
	}
	bindings := map[string]bool{}
	for _, binding := range config.Bindings {
		if binding.ID == "" || bindings[binding.ID] || !accounts[binding.AccountID] || binding.ChannelID <= 0 || binding.UpstreamGroup == "" {
			return nil, errors.New("Invalid monitor binding")
		}
		bindings[binding.ID] = true
	}
	scenario := config.Scenario
	if scenario.Currency == "" || !monitorFinite(scenario.USDExchange) || scenario.USDExchange == 0 || !monitorFinite(scenario.RevenueFactor) || scenario.RevenueFactor == 0 || !monitorFee(scenario.FeeBearer, scenario.FeeRate) {
		return nil, errors.New("Invalid monitor scenario")
	}
	if err := validateMonitorScenarioQuantities(scenario); err != nil {
		return nil, err
	}
	return sites, nil
}

func monitorSensitiveKey(key string) bool {
	key = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	for _, secret := range []string{"password", "passwd", "authorization", "cookie", "apikey", "secret", "accesstoken", "refreshtoken", "idtoken", "csrftoken", "sessionid", "privatekey", "credential"} {
		if strings.Contains(key, secret) {
			return true
		}
	}
	return key == "token" || key == "key" || key == "auth" || key == "session" || key == "jwt"
}
