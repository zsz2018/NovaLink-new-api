package service

import (
	"errors"
	"math"
	"slices"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

type MonitorProfitSelection struct {
	BindingID   string `form:"binding_id"`
	LocalGroup  string `form:"local_group"`
	LocalModel  string `form:"local_model"`
	InputTokens *int   `form:"input_tokens"`
}

type MonitorProfitSide struct {
	Currency      string              `json:"currency"`
	Unit          string              `json:"unit,omitempty"`
	Tier          string              `json:"tier,omitempty"`
	Expression    string              `json:"expression,omitempty"`
	TierMode      string              `json:"tier_mode,omitempty"`
	TierBasis     string              `json:"tier_basis,omitempty"`
	Tiers         []MonitorPriceTier  `json:"tiers,omitempty"`
	TokenUnit     *float64            `json:"token_unit,omitempty"`
	BasePrices    *MonitorPriceTier   `json:"base_prices,omitempty"`
	IncludesGroup *bool               `json:"includes_group,omitempty"`
	BaseAmount    *float64            `json:"base_amount"`
	GroupRatio    *float64            `json:"group_ratio"`
	FXRate        float64             `json:"fx_rate"`
	Factor        float64             `json:"factor"`
	FeeRate       float64             `json:"fee_rate"`
	FeeBearer     string              `json:"fee_bearer"`
	FeeMultiplier *float64            `json:"fee_multiplier"`
	Amount        *float64            `json:"amount"`
	Estimated     bool                `json:"estimated"`
	PriceSource   *MonitorPriceSource `json:"price_source,omitempty"`
}

func (side MonitorProfitSide) convert(value float64) float64 {
	if side.GroupRatio == nil || side.FeeMultiplier == nil {
		return math.NaN()
	}
	return value * *side.GroupRatio * side.FXRate * side.Factor * *side.FeeMultiplier
}

func monitorProfitFactors(row MonitorProfitRow, scenario MonitorScenario, account MonitorAccount, site MonitorSite, price *MonitorModelPrice) (MonitorProfitSide, MonitorProfitSide) {
	local := MonitorProfitSide{Currency: "USD", GroupRatio: &row.LocalRatio, FXRate: scenario.USDExchange, Factor: scenario.RevenueFactor, FeeRate: scenario.FeeRate, FeeBearer: scenario.FeeBearer}
	upstream := MonitorProfitSide{Currency: site.Currency, FXRate: site.FXRate, Factor: account.PurchaseFactor, FeeRate: account.FeeRate, FeeBearer: account.FeeBearer}
	if price != nil {
		upstream.Estimated, upstream.PriceSource = price.PriceSource != nil, price.PriceSource
	}
	if scenario.FeeBearer != "unknown" {
		fee := 1.0
		if scenario.FeeBearer == "self" {
			fee -= scenario.FeeRate
		}
		local.FeeMultiplier = &fee
	}
	if account.FeeBearer != "unknown" {
		fee := 1.0
		if account.FeeBearer == "self" {
			fee += account.FeeRate
		}
		upstream.FeeMultiplier = &fee
	}
	if price != nil && price.IncludesGroup != nil {
		if *price.IncludesGroup {
			upstream.GroupRatio = new(1.0)
		} else {
			upstream.GroupRatio = row.UpstreamRatio
		}
	}
	return local, upstream
}

type MonitorProfitComponent struct {
	Kind              string   `json:"kind"`
	Quantity          int      `json:"quantity"`
	LocalUnitPrice    *float64 `json:"local_unit_price"`
	UpstreamUnitPrice *float64 `json:"upstream_unit_price"`
	UnitProfit        *float64 `json:"unit_profit"`
	UnitMargin        *float64 `json:"unit_margin"`
	Revenue           *float64 `json:"revenue"`
	Cost              *float64 `json:"cost"`
}

func (part *MonitorProfitComponent) compareUnitPrices() {
	if part.LocalUnitPrice == nil || part.UpstreamUnitPrice == nil {
		return
	}
	profit := *part.LocalUnitPrice - *part.UpstreamUnitPrice
	if math.IsInf(profit, 0) || math.IsNaN(profit) {
		return
	}
	part.UnitProfit = &profit
	if *part.LocalUnitPrice > 0 {
		margin := profit / *part.LocalUnitPrice
		if !math.IsInf(margin, 0) && !math.IsNaN(margin) {
			part.UnitMargin = &margin
		}
	}
}

type MonitorProfitNormalized struct {
	Revenue float64  `json:"revenue"`
	Cost    float64  `json:"cost"`
	Profit  float64  `json:"profit"`
	Margin  *float64 `json:"margin"`
}

type MonitorProfitTierSample struct {
	InputTokens   int                      `json:"input_tokens"`
	ContextTokens int                      `json:"context_tokens"`
	LocalTier     string                   `json:"local_tier"`
	UpstreamTier  string                   `json:"upstream_tier"`
	Revenue       *float64                 `json:"revenue"`
	Cost          *float64                 `json:"cost"`
	Profit        *float64                 `json:"profit"`
	Margin        *float64                 `json:"margin"`
	Status        string                   `json:"status"`
	Reason        string                   `json:"reason"`
	Components    []MonitorProfitComponent `json:"components,omitempty"`
}

type MonitorProfitDetail struct {
	Row               MonitorProfitRow          `json:"row"`
	Scenario          MonitorScenario           `json:"scenario"`
	ContextTokens     int                       `json:"context_tokens"`
	Local             *MonitorProfitSide        `json:"local,omitempty"`
	Upstream          *MonitorProfitSide        `json:"upstream,omitempty"`
	Components        []MonitorProfitComponent  `json:"components"`
	TierSamples       []MonitorProfitTierSample `json:"tier_samples,omitempty"`
	PerMillionContext *MonitorProfitNormalized  `json:"per_million_context,omitempty"`
}

func GetMonitorProfitDetail(selection MonitorProfitSelection) (MonitorProfitDetail, error) {
	if selection.BindingID == "" || len(selection.BindingID) > 128 || selection.LocalGroup == "" || len(selection.LocalGroup) > 255 || selection.LocalModel == "" || len(selection.LocalModel) > 255 {
		return MonitorProfitDetail{}, errors.New("Invalid monitor request")
	}
	config, err := GetMonitorConfig()
	if err != nil {
		return MonitorProfitDetail{}, err
	}
	if selection.InputTokens != nil {
		config.Scenario.InputTokens = *selection.InputTokens
	}
	if err := validateMonitorScenarioQuantities(config.Scenario); err != nil {
		return MonitorProfitDetail{}, err
	}
	index := slices.IndexFunc(config.Bindings, func(binding MonitorBinding) bool { return binding.ID == selection.BindingID })
	if index < 0 {
		return MonitorProfitDetail{}, errors.New("Procurement route is no longer available")
	}
	binding := config.Bindings[index]
	config.Bindings = []MonitorBinding{binding}
	var snapshots []model.MonitorSnapshot
	if err := model.DB.Where("account_id = ? AND confirmed = ? AND complete = ?", binding.AccountID, true, true).Order("created_at desc, id desc").Limit(1).Find(&snapshots).Error; err != nil {
		return MonitorProfitDetail{}, err
	}
	var detail MonitorProfitDetail
	rows, err := monitorProfitRows(config, snapshots, &selection, &detail)
	if err != nil {
		return detail, err
	}
	if len(rows) != 1 {
		return detail, errors.New("Procurement route is no longer available")
	}
	return detail, nil
}

func monitorKnownAmount(value float64) *float64 {
	if !monitorFinite(value) {
		return nil
	}
	return &value
}

func monitorExplainProfit(row MonitorProfitRow, scenario MonitorScenario, account MonitorAccount, site MonitorSite, price *MonitorModelPrice, local model.ModelPricingEntry) MonitorProfitDetail {
	expression, _ := monitorLocalExpression(local)
	detail := monitorProfitScenarioDetail(row, scenario, account, site, price, expression)
	if price != nil {
		detail.TierSamples = monitorProfitTierSamples(row, scenario, account, site, *price, local, expression, true)
	}
	return detail
}

func monitorProfitScenarioDetail(row MonitorProfitRow, scenario MonitorScenario, account MonitorAccount, site MonitorSite, price *MonitorModelPrice, expression string) MonitorProfitDetail {
	detail := MonitorProfitDetail{Row: row, Scenario: scenario, ContextTokens: scenario.InputTokens + scenario.CacheRead + scenario.CacheWrite, Components: []MonitorProfitComponent{}}
	sale, purchase := monitorProfitFactors(row, scenario, account, site, price)
	var saleRates, purchaseRates [4]*float64
	sale.Expression = expression
	if expression != "" {
		value, trace, evalErr := monitorScenarioExpression(expression, scenario, false)
		if evalErr == nil {
			sale.BaseAmount = monitorKnownAmount(value)
			sale.Amount = monitorKnownAmount(sale.convert(value))
			sale.Unit, sale.Tier = string(trace.BillingUnit), trace.MatchedTier
			saleRates = monitorExpressionUnitPrices(expression, scenario, trace, value)
		}
	}
	detail.Local = &sale
	if price != nil {
		purchase.Currency = price.Currency
		purchase.Expression, purchase.Unit = price.BillingExpression, price.BillingUnit
		purchase.TierMode, purchase.TierBasis, purchase.Tiers = price.TierMode, price.TierBasis, price.Tiers
		purchase.IncludesGroup = price.IncludesGroup
		if price.BillingUnit == "token" {
			purchase.TokenUnit = price.TokenUnit
			purchase.BasePrices = &MonitorPriceTier{Input: price.Input, Output: price.Output, CacheRead: price.CacheRead, CacheWrite: price.CacheWrite}
		}
		value, tier, err := monitorUpstreamPrice(*price, scenario)
		if err == nil {
			purchase.Tier = tier
			purchase.BaseAmount = monitorKnownAmount(value)
			if price.Currency == site.Currency {
				purchase.Amount = monitorKnownAmount(purchase.convert(value))
			}
			if price.BillingUnit == "expression" && price.Currency == site.Currency {
				_, trace, err := monitorScenarioExpression(price.BillingExpression, scenario, true)
				if err == nil {
					purchase.Unit = string(trace.BillingUnit)
					purchaseRates = monitorExpressionUnitPrices(price.BillingExpression, scenario, trace, value)
				}
			}
		}
		if price.BillingUnit == "token" && !price.PricingPending && price.Currency == site.Currency && price.TokenUnit != nil && *price.TokenUnit > 0 && validateMonitorPriceTiers(*price) == nil {
			rates, tier, _, err := monitorUpstreamTokenPrices(*price, scenario)
			purchase.Tier = tier
			if err != nil {
				rates[0] = nil
			}
			for i, rate := range rates {
				if rate != nil {
					purchaseRates[i] = monitorKnownAmount(*rate / *price.TokenUnit * 1_000_000)
				}
			}
		}
		detail.Upstream = &purchase
	}
	kinds := []string{"input", "output", "cache_read", "cache_write"}
	quantities := []int{scenario.InputTokens, scenario.OutputTokens, scenario.CacheRead, scenario.CacheWrite}
	for i, kind := range kinds {
		part := MonitorProfitComponent{Kind: kind, Quantity: quantities[i]}
		if sale.Unit == "request" {
			part.Revenue = new(0.0)
		} else if saleRates[i] != nil {
			part.LocalUnitPrice = monitorKnownAmount(sale.convert(*saleRates[i]))
			if part.LocalUnitPrice != nil {
				part.Revenue = monitorKnownAmount(*part.LocalUnitPrice / 1_000_000 * float64(quantities[i]) * float64(scenario.Requests))
			}
		}
		if purchase.Unit == "request" && purchase.BaseAmount != nil {
			part.Cost = new(0.0)
		} else if purchaseRates[i] != nil {
			part.UpstreamUnitPrice = monitorKnownAmount(purchase.convert(*purchaseRates[i]))
			if part.UpstreamUnitPrice != nil {
				part.Cost = monitorKnownAmount(*part.UpstreamUnitPrice / 1_000_000 * float64(quantities[i]) * float64(scenario.Requests))
			}
		}
		part.compareUnitPrices()
		detail.Components = append(detail.Components, part)
	}
	if sale.Unit == "request" || purchase.Unit == "request" {
		part := MonitorProfitComponent{Kind: "request", Quantity: scenario.Requests}
		if sale.Unit == "request" {
			if sale.Amount != nil {
				part.Revenue = sale.Amount
				part.LocalUnitPrice = monitorKnownAmount(*sale.Amount / float64(scenario.Requests))
			}
		} else if sale.Amount != nil {
			part.Revenue = new(0.0)
		}
		if purchase.Unit == "request" {
			if purchase.Amount != nil {
				part.Cost = purchase.Amount
				part.UpstreamUnitPrice = monitorKnownAmount(*purchase.Amount / float64(scenario.Requests))
			}
		} else if purchase.Amount != nil {
			part.Cost = new(0.0)
		}
		part.compareUnitPrices()
		detail.Components = append(detail.Components, part)
	}
	if detail.ContextTokens > 0 && row.Status == "ready" && row.Revenue != nil && row.Cost != nil && row.Profit != nil {
		factor := 1_000_000 / (float64(detail.ContextTokens) * float64(scenario.Requests))
		normalized := MonitorProfitNormalized{Revenue: *row.Revenue * factor, Cost: *row.Cost * factor, Profit: *row.Profit * factor, Margin: row.Margin}
		if monitorFinite(normalized.Revenue) && monitorFinite(normalized.Cost) && !math.IsInf(normalized.Profit, 0) && !math.IsNaN(normalized.Profit) {
			detail.PerMillionContext = &normalized
		}
	}
	return detail
}

// Both sides use the same input length and the saved output, cache and request
// quantities. Boundary candidates determine the inputs, while the billing
// engine remains responsible for selecting and evaluating each pricing tier.
func monitorProfitTierSamples(row MonitorProfitRow, scenario MonitorScenario, account MonitorAccount, site MonitorSite, price MonitorModelPrice, local model.ModelPricingEntry, localExpression string, includeComponents bool) []MonitorProfitTierSample {
	if localExpression != "" {
		// Reuse the resolved expression for legacy pricing as well.
		local.Effective = model.PricingValues{"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": localExpression}
	}
	boundaries := map[int]bool{}
	cache := scenario.CacheRead + scenario.CacheWrite
	addBoundary := func(boundary int) {
		if boundary >= 0 && boundary < maxMonitorScenarioTokens {
			boundaries[boundary] = true
		}
	}
	for _, tier := range price.Tiers {
		if tier.InputThreshold != nil && *tier.InputThreshold <= int64(maxMonitorScenarioTokens+cache) {
			boundary := int(*tier.InputThreshold)
			if price.TierBasis == "context" {
				boundary -= cache
			}
			addBoundary(boundary)
		}
	}
	for _, expression := range []string{localExpression, price.BillingExpression} {
		if validateMonitorPriceExpression(expression) != nil {
			continue
		}
		_, body := billingexpr.ParseExprVersion(expression)
		tree, err := parser.Parse(body)
		if err != nil {
			continue
		}
		vars := billingexpr.UsedVars(expression)
		ast.Find(tree.Node, func(node ast.Node) bool {
			comparison, ok := node.(*ast.BinaryNode)
			if !ok || !slices.Contains([]string{"<", "<=", ">", ">=", "==", "!="}, comparison.Operator) {
				return false
			}
			identifier, literal := comparison.Left, comparison.Right
			limitBefore := comparison.Operator == "<" || comparison.Operator == ">="
			if _, ok := identifier.(*ast.IdentifierNode); !ok {
				identifier, literal = literal, identifier
				limitBefore = comparison.Operator == ">" || comparison.Operator == "<="
			}
			variable, ok := identifier.(*ast.IdentifierNode)
			if !ok || variable.Value != "len" && variable.Value != "p" {
				return false
			}
			var value float64
			switch literal := literal.(type) {
			case *ast.IntegerNode:
				value = float64(literal.Value)
			case *ast.FloatNode:
				value = literal.Value
			default:
				return false
			}
			if !monitorFinite(value) || value > float64(maxMonitorScenarioTokens+cache) {
				return false
			}
			boundary := int(math.Floor(value))
			if variable.Value == "len" {
				boundary -= cache
			} else {
				if !vars["cr"] {
					boundary -= scenario.CacheRead
				}
				if !vars["cc"] {
					boundary -= scenario.CacheWrite
				}
			}
			if comparison.Operator == "==" || comparison.Operator == "!=" {
				addBoundary(boundary - 1)
				addBoundary(boundary)
			} else if limitBefore && value == math.Floor(value) {
				addBoundary(boundary - 1)
			} else {
				addBoundary(boundary)
			}
			return false
		})
	}
	if len(boundaries) == 0 || len(boundaries) > 64 {
		return nil
	}
	ordered := make([]int, 0, len(boundaries))
	for boundary := range boundaries {
		ordered = append(ordered, boundary)
	}
	slices.Sort(ordered)
	inputs := make([]int, 0, len(ordered)+1)
	if includeComponents {
		for _, boundary := range ordered {
			if len(inputs) == 0 || inputs[len(inputs)-1] != boundary {
				inputs = append(inputs, boundary)
			}
			inputs = append(inputs, boundary+1)
		}
		if len(inputs) > 64 {
			return nil
		}
	} else {
		// One trial per interval, using the saved input in its own interval.
		// Retain distinct intervals even when expressions reuse a tier label.
		lower := 0
		for _, upper := range append(ordered, maxMonitorScenarioTokens) {
			input := lower
			if lower == 0 {
				input = upper
			}
			if scenario.InputTokens >= lower && scenario.InputTokens <= upper {
				input = scenario.InputTokens
			}
			inputs = append(inputs, input)
			lower = upper + 1
		}
	}
	samples := make([]MonitorProfitTierSample, 0, len(inputs))
	for _, input := range inputs {
		trial := scenario
		trial.InputTokens = input
		result := row
		if input != scenario.InputTokens {
			result = MonitorProfitRow{LocalRatio: row.LocalRatio, UpstreamRatio: row.UpstreamRatio, Status: "pending"}
			monitorCalculateProfit(&result, trial, account, site, price, local)
		}
		sample := MonitorProfitTierSample{
			InputTokens: input, ContextTokens: input + cache,
			LocalTier: result.LocalTier, UpstreamTier: result.UpstreamTier,
			Revenue: result.Revenue, Cost: result.Cost, Profit: result.Profit, Margin: result.Margin,
			Status: result.Status, Reason: result.Reason,
		}
		if includeComponents || result.Status != "ready" {
			breakdown := monitorProfitScenarioDetail(result, trial, account, site, &price, localExpression)
			sample.LocalTier, sample.UpstreamTier = breakdown.Local.Tier, breakdown.Upstream.Tier
			if includeComponents {
				sample.Components = breakdown.Components
			}
		}
		samples = append(samples, sample)
	}
	return samples
}

// Explain only affine token prices. Conditions are selected by the billing
// engine with the same normalized quantities as the original expression.
// Nonlinear prices remain unknown; sampling is never presented as a unit price.
func monitorExpressionUnitPrices(expression string, scenario MonitorScenario, trace billingexpr.TraceResult, amount float64) [4]*float64 {
	var prices [4]*float64
	if trace.BillingUnit != "token" {
		return prices
	}
	_, body := billingexpr.ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return prices
	}
	vars := billingexpr.UsedVars(expression)
	params := monitorExpressionParams(expression, scenario)
	coefficients, ok := monitorAffinePrice(tree.Node, params)
	if !ok || coefficients[4] != 0 {
		return prices
	}
	if !vars["cr"] {
		coefficients[2] = coefficients[0]
	}
	if !vars["cc"] {
		coefficients[3] = coefficients[0]
	}
	predicted := 0.0
	quantities := []int{scenario.InputTokens, scenario.OutputTokens, scenario.CacheRead, scenario.CacheWrite}
	for i := range 4 {
		if !monitorFinite(coefficients[i]) {
			return prices
		}
		predicted += coefficients[i] * float64(quantities[i])
	}
	evaluated := amount / float64(scenario.Requests) * 1_000_000
	if !monitorFinite(predicted) || math.Abs(predicted-evaluated) > 1e-7*max(1, math.Abs(evaluated)) {
		return prices
	}
	for i := range prices {
		prices[i] = new(coefficients[i])
	}
	return prices
}

// Coefficients are p,c,cr,cc and the constant term, in USD per million tokens.
func monitorAffinePrice(node ast.Node, params billingexpr.TokenParams) ([5]float64, bool) {
	var value [5]float64
	switch node := node.(type) {
	case *ast.IntegerNode:
		value[4] = float64(node.Value)
		return value, true
	case *ast.FloatNode:
		value[4] = node.Value
		return value, true
	case *ast.IdentifierNode:
		switch node.Value {
		case "p":
			value[0] = 1
		case "c":
			value[1] = 1
		case "cr":
			value[2] = 1
		case "cc":
			value[3] = 1
		case "cc1h":
			value[4] = params.CC1h
		case "cc12h":
			value[4] = params.CC12h
		default:
			return value, false
		}
		return value, true
	case *ast.UnaryNode:
		value, ok := monitorAffinePrice(node.Node, params)
		if !ok {
			return value, false
		}
		if node.Operator == "-" {
			for i := range value {
				value[i] = -value[i]
			}
			return value, true
		}
		return value, node.Operator == "+"
	case *ast.CallNode:
		name, ok := node.Callee.(*ast.IdentifierNode)
		if ok && name.Value == "tier" && len(node.Arguments) == 2 {
			return monitorAffinePrice(node.Arguments[1], params)
		}
	case *ast.ConditionalNode:
		condition := node.Cond.String() + ` ? tier("true", p * 0) : tier("false", p * 0)`
		_, trace, err := billingexpr.RunExpr(condition, params)
		if err != nil {
			return value, false
		}
		if trace.MatchedTier == "true" {
			return monitorAffinePrice(node.Exp1, params)
		}
		return monitorAffinePrice(node.Exp2, params)
	case *ast.BinaryNode:
		left, lok := monitorAffinePrice(node.Left, params)
		right, rok := monitorAffinePrice(node.Right, params)
		if !lok || !rok {
			return value, false
		}
		switch node.Operator {
		case "+", "-":
			for i := range value {
				if node.Operator == "+" {
					value[i] = left[i] + right[i]
				} else {
					value[i] = left[i] - right[i]
				}
			}
			return value, true
		case "*", "/":
			leftConstant, rightConstant := true, true
			for i := range 4 {
				leftConstant = leftConstant && left[i] == 0
				rightConstant = rightConstant && right[i] == 0
			}
			if node.Operator == "*" && leftConstant {
				for i := range value {
					value[i] = right[i] * left[4]
				}
				return value, true
			}
			if !rightConstant || node.Operator == "/" && right[4] == 0 {
				return value, false
			}
			for i := range value {
				if node.Operator == "*" {
					value[i] = left[i] * right[4]
				} else {
					value[i] = left[i] / right[4]
				}
			}
			return value, true
		}
	}
	return value, false
}
