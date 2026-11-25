package service

import (
	"bytes"
	_ "embed"
	"errors"
	"net/url"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

// Application updates append catalog versions; existing policies keep their
// pinned version. The registry never consults gateway selling-price settings.
//
//go:embed monitor_official_catalog.json
var monitorOfficialCatalogJSON []byte

type monitorOfficialCatalogRegistry struct {
	current  string
	versions map[string]MonitorOfficialCatalog
}

var monitorOfficialCatalogs = sync.OnceValues(func() (monitorOfficialCatalogRegistry, error) {
	return loadMonitorOfficialCatalogRegistry(monitorOfficialCatalogJSON)
})

func loadMonitorOfficialCatalogRegistry(data []byte) (monitorOfficialCatalogRegistry, error) {
	var source struct {
		Catalogs []MonitorOfficialCatalog `json:"catalogs"`
	}
	registry := monitorOfficialCatalogRegistry{versions: map[string]MonitorOfficialCatalog{}}
	if err := common.DecodeJsonBodyStrict(bytes.NewReader(data), &source); err != nil || len(source.Catalogs) == 0 {
		return registry, errors.New("Monitor official catalog is unavailable")
	}
	for _, catalog := range source.Catalogs {
		if !validMonitorOfficialIdentity(catalog.Version) || !validMonitorOfficialIdentity(catalog.VerifiedAt) || len(catalog.Entries) == 0 {
			return registry, errors.New("Monitor official catalog is unavailable")
		}
		if _, exists := registry.versions[catalog.Version]; exists {
			return registry, errors.New("Monitor official catalog is unavailable")
		}
		seen := map[string]bool{}
		for _, entry := range catalog.Entries {
			quote := entry.Quote
			parsed, err := url.Parse(entry.SourceURL)
			if !validMonitorOfficialIdentity(entry.ID) || !validMonitorOfficialIdentity(entry.Provider) || !validMonitorOfficialIdentity(entry.Model) || entry.ID != entry.Provider+":"+entry.Model || !validMonitorOfficialIdentity(entry.VerifiedAt) || seen[entry.ID] || err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || len(entry.SourceURL) > 2048 || strings.IndexFunc(entry.SourceURL, unicode.IsControl) >= 0 || quote.Currency != "USD" || (quote.BillingUnit != "token" && quote.BillingUnit != "expression") || quote.TokenUnit == nil || *quote.TokenUnit != 1_000_000 || quote.IncludesGroup == nil || *quote.IncludesGroup || quote.Input == nil || quote.Output == nil || quote.RequestPrice != nil || quote.PricingPending || quote.PriceSource != nil || len(quote.Notes) > 512 {
				return registry, errors.New("Monitor official catalog is unavailable")
			}
			if quote.BillingUnit == "expression" && validateMonitorPriceExpression(quote.BillingExpression) != nil || quote.BillingUnit == "token" && quote.BillingExpression != "" {
				return registry, errors.New("Monitor official catalog is unavailable")
			}
			if validateMonitorPriceTiers(quote) != nil {
				return registry, errors.New("Monitor official catalog is unavailable")
			}
			for _, tier := range quote.Tiers {
				if tier.Input == nil || quote.TierMode == "whole_request" && tier.Output == nil {
					return registry, errors.New("Monitor official catalog is unavailable")
				}
			}
			for _, number := range []*float64{quote.Input, quote.Output, quote.CacheRead, quote.CacheWrite} {
				if number != nil && (!monitorFinite(*number) || *number > 1e15) {
					return registry, errors.New("Monitor official catalog is unavailable")
				}
			}
			seen[entry.ID] = true
		}
		registry.versions[catalog.Version] = catalog
		registry.current = catalog.Version
	}
	return registry, nil
}

// GetMonitorOfficialCatalog returns an independent copy of the selected catalog.
// An empty version selects current; a missing pinned version never falls back.
func GetMonitorOfficialCatalog(version string) (*MonitorOfficialCatalog, error) {
	registry, err := monitorOfficialCatalogs()
	if err != nil {
		return nil, err
	}
	if version == "" {
		version = registry.current
	}
	catalog, exists := registry.versions[version]
	if !exists {
		return nil, errors.New("Monitor official catalog is unavailable")
	}
	encoded, err := common.Marshal(catalog)
	if err != nil {
		return nil, err
	}
	var copy MonitorOfficialCatalog
	if err := common.Unmarshal(encoded, &copy); err != nil {
		return nil, err
	}
	return &copy, nil
}

// Dashboard comparison settings expose current plus all distinct account-pinned
// versions. A catalog update never rewrites an account's selected version.
func monitorComparisonCatalogs(accounts []MonitorAccount) ([]MonitorOfficialCatalog, string, error) {
	current, err := GetMonitorOfficialCatalog("")
	if err != nil {
		return nil, "", err
	}
	catalogs := []MonitorOfficialCatalog{*current}
	seen := map[string]bool{current.Version: true}
	for _, account := range accounts {
		if account.OfficialEstimate == nil || seen[account.OfficialEstimate.CatalogVersion] {
			continue
		}
		catalog, err := GetMonitorOfficialCatalog(account.OfficialEstimate.CatalogVersion)
		if err != nil {
			return nil, "", err
		}
		catalogs = append(catalogs, *catalog)
		seen[catalog.Version] = true
	}
	return catalogs, current.Version, nil
}

func validMonitorOfficialIdentity(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= 255 && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0 && !strings.Contains(strings.ToLower(value), "[redacted]")
}

func validateMonitorOfficialEstimate(policy *MonitorOfficialEstimatePolicy, site MonitorSite) error {
	if policy == nil {
		return nil
	}
	if site.Currency != "USD" || !validMonitorOfficialIdentity(policy.CatalogVersion) || len(policy.Models) < 1 || len(policy.Models) > 500 {
		return errors.New("Invalid monitor official estimate policy")
	}
	catalog, err := GetMonitorOfficialCatalog(policy.CatalogVersion)
	if err != nil {
		return err
	}
	entries := make(map[string]bool, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		entries[entry.ID] = true
	}
	seen := map[string]bool{}
	for _, selection := range policy.Models {
		key := selection.Group + "\x00" + selection.Name
		if !validMonitorOfficialIdentity(selection.Name) || !validMonitorOfficialIdentity(selection.Group) || len(selection.Group) > 128 || !validMonitorOfficialIdentity(selection.EntryID) || !entries[selection.EntryID] || seen[key] {
			return errors.New("Invalid monitor official estimate policy")
		}
		seen[key] = true
	}
	return nil
}

// An empty model row has identity/unit metadata but no captured pricing facts.
// A zero price, a partial price set, unresolved rules or an unsupported unit is
// meaningful captured data and must never be replaced with an official quote.
func monitorPriceWhollyEmpty(price MonitorModelPrice) bool {
	return (price.BillingUnit == "" || price.BillingUnit == "token") && price.Input == nil && price.Output == nil && price.CacheRead == nil && price.CacheWrite == nil && price.RequestPrice == nil && price.BillingExpression == "" && len(price.Tiers) == 0 && price.TierMode == "" && price.TierBasis == "" && !price.PricingPending && price.PriceSource == nil && strings.TrimSpace(price.Notes) == ""
}

// Resolve a comparison quote without mutating data.Models or any snapshot. The
// account policy is consulted only after the matching captured row is absent or
// wholly unpriced, and only against a known captured money-billing group.
func monitorComparisonPrice(data MonitorData, account MonitorAccount, site MonitorSite, name, groupName string) (*MonitorModelPrice, string) {
	var captured *MonitorModelPrice
	for i := range data.Models {
		price := &data.Models[i]
		if price.Name == name && price.Group == groupName {
			captured = price
			break
		}
	}
	if captured != nil && (!monitorPriceWhollyEmpty(*captured) || captured.Currency != site.Currency) {
		return captured, ""
	}
	policy := account.OfficialEstimate
	if policy == nil {
		return captured, "Upstream model price is missing"
	}
	var selection *MonitorOfficialEstimateModel
	for i := range policy.Models {
		if policy.Models[i].Name == name && policy.Models[i].Group == groupName {
			selection = &policy.Models[i]
			break
		}
	}
	if selection == nil {
		return captured, "Upstream model price is missing"
	}
	if err := validateMonitorOfficialEstimate(policy, site); err != nil {
		return nil, err.Error()
	}
	var group *MonitorGroup
	for i := range data.Groups {
		if data.Groups[i].Name == groupName {
			group = &data.Groups[i]
			break
		}
	}
	if group == nil || group.Ratio == nil || !monitorFinite(*group.Ratio) || *group.Ratio > 1e15 {
		return nil, "Upstream group ratio is unknown"
	}
	if group.CostUnsupported != "" {
		return nil, "Upstream group cost comparison is unsupported"
	}
	catalog, err := GetMonitorOfficialCatalog(policy.CatalogVersion)
	if err != nil {
		return nil, err.Error()
	}
	for _, entry := range catalog.Entries {
		if entry.ID != selection.EntryID {
			continue
		}
		quote := entry.Quote
		quote.Name, quote.Group = name, groupName
		quote.PriceSource = &MonitorPriceSource{Kind: "official_estimate", CatalogVersion: catalog.Version, EntryID: entry.ID, Provider: entry.Provider, Model: entry.Model, SourceURL: entry.SourceURL, VerifiedAt: entry.VerifiedAt}
		return &quote, ""
	}
	return nil, "Invalid monitor official estimate policy"
}
