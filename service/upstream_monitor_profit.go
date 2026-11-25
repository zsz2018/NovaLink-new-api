package service

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func MonitorState() (map[string]any, error) {
	config, err := GetMonitorConfig()
	if err != nil {
		return nil, err
	}
	devices := []model.MonitorDevice{}
	jobs := []model.MonitorJob{}
	snapshots := []model.MonitorSnapshot{}
	channels := []model.Channel{}
	if err := model.DB.Order("created_at desc, id desc").Find(&devices).Error; err != nil {
		return nil, err
	}
	if err := model.DB.Order("created_at desc, id desc").Limit(100).Find(&jobs).Error; err != nil {
		return nil, err
	}
	if err := model.DB.Order("created_at desc, id desc").Limit(200).Find(&snapshots).Error; err != nil {
		return nil, err
	}
	if err := model.DB.Select("id", "name", commonGroupColumnMonitor(), "models", "model_mapping").Find(&channels).Error; err != nil {
		return nil, err
	}
	channelRows := make([]map[string]any, 0, len(channels))
	for _, channel := range channels {
		channelRows = append(channelRows, map[string]any{"id": channel.Id, "name": channel.Name, "group": channel.Group, "models": channel.Models, "model_mapping": channel.ModelMapping})
	}
	snapshotRows := make([]map[string]any, 0, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotRows = append(snapshotRows, monitorSnapshotResponse(snapshot))
	}
	confirmed := []model.MonitorSnapshot{}
	if err := model.DB.Where("confirmed = ? AND complete = ?", true, true).Order("created_at desc, id desc").Find(&confirmed).Error; err != nil {
		return nil, err
	}
	rows, err := monitorProfitRows(config, confirmed, nil, nil)
	if err != nil {
		return nil, err
	}
	catalogs, latestVersion, err := monitorComparisonCatalogs(config.Accounts)
	if err != nil {
		return nil, err
	}
	return map[string]any{"config": config, "devices": devices, "jobs": jobs, "snapshots": snapshotRows, "channels": channelRows, "groups": ratio_setting.GetGroupRatioCopy(), "profit_rows": rows, "key_ready": MonitorKeyReady(), "official_catalogs": catalogs, "official_catalog_latest_version": latestVersion}, nil
}

func commonGroupColumnMonitor() string {
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return `"group"`
	}
	return "`group`"
}

func monitorProfitRows(config MonitorConfig, snapshots []model.MonitorSnapshot, selection *MonitorProfitSelection, detail *MonitorProfitDetail) ([]MonitorProfitRow, error) {
	rows := []MonitorProfitRow{}
	latest := map[string]model.MonitorSnapshot{}
	for _, snapshot := range snapshots {
		if snapshot.Confirmed && snapshot.Complete {
			if _, exists := latest[snapshot.AccountID]; !exists {
				latest[snapshot.AccountID] = snapshot
			}
		}
	}
	accounts := map[string]MonitorAccount{}
	sites := map[string]MonitorSite{}
	for _, account := range config.Accounts {
		accounts[account.ID] = account
	}
	for _, site := range config.Sites {
		sites[site.ID] = site
	}
	abilities, err := model.GetAllEnableAbilityWithChannels()
	if err != nil {
		return nil, err
	}
	channels := map[int]model.Channel{}
	var channelList []model.Channel
	if err := model.DB.Select("id", "name", "model_mapping", "status").Find(&channelList).Error; err != nil {
		return nil, err
	}
	for _, channel := range channelList {
		channels[channel.Id] = channel
	}
	pricingNames := []string{}
	for _, ability := range abilities {
		if selection == nil || (ability.Group == selection.LocalGroup && ability.Model == selection.LocalModel) {
			pricingNames = append(pricingNames, ability.Model)
		}
	}
	if selection != nil && len(pricingNames) == 0 {
		return rows, nil
	}
	pricing, err := model.GetModelPricingSnapshot(pricingNames)
	if err != nil {
		return nil, err
	}
	entries := map[string]model.ModelPricingEntry{}
	for _, entry := range pricing.Entries {
		entries[entry.ModelName] = entry
	}
	for _, ability := range abilities {
		if selection != nil && (ability.Group != selection.LocalGroup || ability.Model != selection.LocalModel) {
			continue
		}
		channel, exists := channels[ability.ChannelId]
		if !exists || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		matched := false
		for _, binding := range config.Bindings {
			if selection != nil && binding.ID != selection.BindingID {
				continue
			}
			if binding.ChannelID != ability.ChannelId || (binding.LocalModel != "" && binding.LocalModel != ability.Model) {
				continue
			}
			matched = true
			account, exists := accounts[binding.AccountID]
			if !exists {
				continue
			}
			site := sites[account.SiteID]
			upstream := monitorBoundModelName(binding, channel, ability.Model)
			ratio := ratio_setting.GetGroupRatio(ability.Group)
			if special, ok := ratio_setting.GetGroupGroupRatio(config.Scenario.UserGroup, ability.Group); config.Scenario.UserGroup != "" && ok {
				ratio = special
			}
			row := MonitorProfitRow{BindingID: binding.ID, LocalGroup: ability.Group, LocalRatio: ratio, ChannelID: channel.Id, ChannelName: channel.Name, AccountID: account.ID, SiteName: site.Name, UpstreamGroup: binding.UpstreamGroup, LocalModel: ability.Model, UpstreamModel: upstream, Status: "pending", Reason: "No confirmed complete upstream snapshot"}
			var matchedPrice *MonitorModelPrice
			snapshot, exists := latest[account.ID]
			if exists {
				row.SnapshotID, row.CapturedAt = snapshot.ID, snapshot.CreatedAt
				var data MonitorData
				if err := common.UnmarshalJsonStr(string(snapshot.Data), &data); err != nil {
					return nil, err
				}
				for _, group := range data.Groups {
					if group.Name == binding.UpstreamGroup {
						row.UpstreamRatio = group.Ratio
						break
					}
				}
				matchedPrice, row.Reason = monitorComparisonPrice(data, account, site, upstream, binding.UpstreamGroup)
				if matchedPrice != nil {
					monitorCalculateProfit(&row, config.Scenario, account, site, *matchedPrice, entries[ability.Model])
				}
			}
			if detail != nil {
				*detail = monitorExplainProfit(row, config.Scenario, account, site, matchedPrice, entries[ability.Model])
			} else if matchedPrice != nil {
				local := entries[ability.Model]
				expression, _ := monitorLocalExpression(local)
				row.TierSamples = monitorProfitTierSamples(row, config.Scenario, account, site, *matchedPrice, local, expression, false)
			}
			rows = append(rows, row)
		}
		if !matched && selection == nil {
			rows = append(rows, MonitorProfitRow{LocalGroup: ability.Group, LocalRatio: ratio_setting.GetGroupRatio(ability.Group), ChannelID: channel.Id, ChannelName: channel.Name, LocalModel: ability.Model, Status: "pending", Reason: "Missing upstream binding"})
		}
	}
	return rows, nil
}

func monitorCalculateProfit(row *MonitorProfitRow, scenario MonitorScenario, account MonitorAccount, site MonitorSite, price MonitorModelPrice, local model.ModelPricingEntry) {
	row.Estimated, row.PriceSource = price.PriceSource != nil, price.PriceSource
	if scenario.FeeBearer == "unknown" || account.FeeBearer == "unknown" {
		row.Reason = "Confirm which party bears the recharge fees"
		return
	}
	if price.PricingPending {
		row.Reason = "Upstream pricing rules need confirmation"
		return
	}
	if price.Currency != site.Currency || price.IncludesGroup == nil {
		row.Reason = "Confirm upstream price currency and whether it includes the group ratio"
		return
	}
	cost, upstreamTier, err := monitorUpstreamPrice(price, scenario)
	if err != nil {
		row.Reason = err.Error()
		return
	}
	revenue, localTier, err := monitorLocalPrice(local, scenario)
	if err != nil {
		row.Reason = err.Error()
		return
	}
	localFactors, upstreamFactors := monitorProfitFactors(*row, scenario, account, site, &price)
	if upstreamFactors.GroupRatio == nil {
		row.Reason = "Upstream group ratio is unknown"
		return
	}
	revenue = localFactors.convert(revenue)
	cost = upstreamFactors.convert(cost)
	if !monitorFinite(revenue) || !monitorFinite(cost) {
		row.Reason = "Scenario amount is outside the supported range"
		return
	}
	profit := revenue - cost
	var margin *float64
	if revenue > 0 {
		ratio := profit / revenue
		if math.IsInf(ratio, 0) {
			row.Reason = "Scenario amount is outside the supported range"
			return
		}
		margin = &ratio
	}
	row.Revenue, row.Cost, row.Profit, row.Margin = &revenue, &cost, &profit, margin
	row.LocalTier, row.UpstreamTier = localTier, upstreamTier
	row.Status, row.Reason = "ready", ""
}

func monitorLocalPrice(entry model.ModelPricingEntry, scenario MonitorScenario) (float64, string, error) {
	expression, err := monitorLocalExpression(entry)
	if err != nil {
		return 0, "", err
	}
	value, trace, err := monitorScenarioExpression(expression, scenario, false)
	return value, trace.MatchedTier, err
}

func monitorLocalExpression(entry model.ModelPricingEntry) (string, error) {
	if entry.ModelName == "" {
		return "", errors.New("Local model pricing is not configured")
	}
	pricing := entry.Effective
	expression, _ := pricing["billing_setting.billing_expr"].(string)
	if pricing["billing_setting.billing_mode"] != "tiered_expr" {
		conversion, err := model.PreviewModelPricingConversion(entry.ModelName, entry.Configured)
		if err != nil {
			return "", err
		}
		if conversion.UnsupportedReason != "" {
			return "", errors.New(conversion.UnsupportedReason)
		}
		expression = conversion.Expression
	}
	if expression == "" {
		return "", errors.New("Local model expression is missing")
	}
	return expression, nil
}
