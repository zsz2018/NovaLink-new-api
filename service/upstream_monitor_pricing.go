package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

var monitorTierFields = []string{"input_threshold", "input", "output", "cache_read", "cache_write"}

// Capture completeness concerns account/balance/group facts and selected source
// responses. Pricing availability is separate, preserving partial rows without
// allowing them to become official-estimate candidates.
func monitorCapturedPricingStatus(prices []MonitorModelPrice) string {
	status := "unavailable"
	complete := len(prices) > 0
	for _, price := range prices {
		if !monitorPriceWhollyEmpty(price) {
			status = "partial"
		}
		valid := !price.PricingPending && price.IncludesGroup != nil && validateMonitorPriceTiers(price) == nil
		switch price.BillingUnit {
		case "token":
			valid = valid && price.TokenUnit != nil && *price.TokenUnit > 0 && price.Input != nil && price.Output != nil
		case "request":
			valid = valid && price.RequestPrice != nil
		case "expression":
			valid = valid && validateMonitorPriceExpression(price.BillingExpression) == nil
		default:
			valid = false
		}
		for _, tier := range price.Tiers {
			valid = valid && tier.Input != nil && (price.TierMode != "whole_request" || tier.Output != nil)
		}
		complete = complete && valid
	}
	if complete {
		return "available"
	}
	return status
}

func validateMonitorTierMapping(mapping MonitorMapping, requests map[string]MonitorRequestRule) error {
	if mapping.Tiers == nil {
		return nil
	}
	tiers := mapping.Tiers
	if mapping.Target != "models" || !validMonitorNestedPointer(tiers.Path) || !slices.Contains([]string{"whole_request", "graduated"}, tiers.Mode) || !slices.Contains([]string{"context", "input"}, tiers.Basis) || tiers.Mode == "graduated" && tiers.Basis != "input" || len(tiers.Fields) < 2 || len(tiers.Fields) > len(monitorTierFields) {
		return errors.New("Invalid upstream price tiers")
	}
	for _, required := range []string{"input_threshold", "input"} {
		if _, ok := tiers.Fields[required]; !ok {
			return errors.New("Invalid upstream price tiers")
		}
	}
	for name, field := range tiers.Fields {
		if !slices.Contains(monitorTierFields, name) || field.Kind != "path" || !validMonitorNestedPointer(field.Path) || field.Source != "" || field.FallbackPath != "" || len(field.Value) != 0 || field.Multiply != nil || field.Divide != nil || field.Scale != nil || tiers.Mode == "graduated" && name != "input" && name != "input_threshold" {
			return errors.New("Invalid upstream price tiers")
		}
		if err := validateMonitorField(field, requests, 0, false, false); err != nil {
			return errors.New("Invalid upstream price tiers")
		}
	}
	return nil
}

func validateMonitorPriceTiers(price MonitorModelPrice) error {
	if len(price.Tiers) == 0 {
		if price.TierMode != "" || price.TierBasis != "" {
			return errors.New("Invalid upstream price tiers")
		}
		return nil
	}
	if len(price.Tiers) > 16 || price.BillingExpression != "" || price.BillingUnit != "token" || !slices.Contains([]string{"whole_request", "graduated"}, price.TierMode) || !slices.Contains([]string{"context", "input"}, price.TierBasis) || price.TierMode == "graduated" && price.TierBasis != "input" {
		return errors.New("Invalid upstream price tiers")
	}
	var previous int64
	for _, tier := range price.Tiers {
		if tier.InputThreshold == nil || *tier.InputThreshold <= previous || *tier.InputThreshold > 1_000_000_000 {
			return errors.New("Invalid upstream price tiers")
		}
		previous = *tier.InputThreshold
		if price.TierMode == "graduated" && (tier.Output != nil || tier.CacheRead != nil || tier.CacheWrite != nil) {
			return errors.New("Invalid upstream price tiers")
		}
		for _, number := range []*float64{tier.Input, tier.Output, tier.CacheRead, tier.CacheWrite} {
			if number != nil && (!monitorFinite(*number) || *number > 1e15) {
				return errors.New("Invalid upstream price tiers")
			}
		}
	}
	return nil
}

// Captured expressions use the existing bounded expression language. Inspect the
// original AST so a dormant branch cannot hide an unsupported scenario input.
func validateMonitorPriceExpression(expression string) error {
	if len(expression) == 0 || len(expression) > 8192 || strings.Contains(strings.ToLower(expression), "[redacted]") || strings.ContainsRune(expression, 0) {
		return errors.New("Invalid upstream billing expression")
	}
	_, body := billingexpr.ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return errors.New("Invalid upstream billing expression")
	}
	nodes := 0
	if ast.Find(tree.Node, func(node ast.Node) bool {
		nodes++
		if nodes > 512 {
			return true
		}
		switch value := node.(type) {
		case *ast.IntegerNode, *ast.FloatNode, *ast.StringNode, *ast.BoolNode, *ast.NilNode, *ast.ConditionalNode:
			return false
		case *ast.IdentifierNode:
			return !slices.Contains([]string{"p", "c", "len", "cr", "cc", "cc1h", "cc12h", "img", "img_cr", "img_o", "ai", "ao", "image_count", "tier", "fixed", "max", "min", "abs", "ceil", "floor", "param", "header", "has", "hour", "minute", "weekday", "month", "day", "u"}, value.Value)
		case *ast.UnaryNode:
			return !slices.Contains([]string{"+", "-", "!", "not"}, value.Operator)
		case *ast.BinaryNode:
			return !slices.Contains([]string{"+", "-", "*", "/", "%", "<", "<=", ">", ">=", "==", "!=", "&&", "and", "||", "or", "contains", "startsWith", "endsWith"}, value.Operator)
		case *ast.CallNode:
			_, ok := value.Callee.(*ast.IdentifierNode)
			return !ok
		case *ast.BuiltinNode:
			return !slices.Contains([]string{"abs", "ceil", "floor", "min", "max"}, value.Name)
		default:
			return true
		}
	}) != nil {
		return errors.New("Invalid upstream billing expression")
	}
	if _, err := billingexpr.CompileFromCache(expression); err != nil {
		return errors.New("Invalid upstream billing expression")
	}
	return nil
}

const maxMonitorScenarioTokens = 100_000_000

func validateMonitorScenarioQuantities(scenario MonitorScenario) error {
	if scenario.Requests < 1 || scenario.Requests > 1_000_000 {
		return errors.New("Invalid scenario request count")
	}
	for _, quantity := range []int{scenario.InputTokens, scenario.OutputTokens, scenario.CacheRead, scenario.CacheWrite} {
		if quantity < 0 || quantity > maxMonitorScenarioTokens {
			return errors.New("Invalid scenario token count")
		}
	}
	return nil
}

func monitorScenarioExpression(expression string, scenario MonitorScenario, upstream bool) (float64, billingexpr.TraceResult, error) {
	unsupportedMessage := "Local billing needs unsupported scenario input"
	evaluationMessage := "Local billing expression could not be evaluated"
	if upstream {
		unsupportedMessage = "Upstream billing needs unsupported scenario input"
		evaluationMessage = "Upstream billing expression could not be evaluated"
	}
	if err := validateMonitorPriceExpression(expression); err != nil {
		return 0, billingexpr.TraceResult{}, errors.New(evaluationMessage)
	}
	_, body := billingexpr.ParseExprVersion(expression)
	tree, _ := parser.Parse(body)
	if ast.Find(tree.Node, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.VariableDeclaratorNode, *ast.MemberNode, *ast.PredicateNode, *ast.ArrayNode, *ast.MapNode:
			return true
		case *ast.IdentifierNode:
			return !slices.Contains([]string{"p", "c", "len", "cr", "cc", "cc1h", "cc12h", "tier", "fixed", "max", "min", "abs", "ceil", "floor"}, value.Value)
		}
		return false
	}) != nil {
		return 0, billingexpr.TraceResult{}, errors.New(unsupportedMessage)
	}
	if err := validateMonitorScenarioQuantities(scenario); err != nil {
		return 0, billingexpr.TraceResult{}, err
	}
	value, trace, err := billingexpr.RunExpr(expression, monitorExpressionParams(expression, scenario))
	if err != nil || !monitorFinite(value) {
		return 0, trace, errors.New(evaluationMessage)
	}
	cost := value / 1_000_000 * float64(scenario.Requests)
	if !monitorFinite(cost) {
		return 0, trace, errors.New("Scenario amount is outside the supported range")
	}
	return cost, trace, nil
}

func monitorExpressionParams(expression string, scenario MonitorScenario) billingexpr.TokenParams {
	vars := billingexpr.UsedVars(expression)
	p := float64(scenario.InputTokens)
	if !vars["cr"] {
		p += float64(scenario.CacheRead)
	}
	if !vars["cc"] {
		p += float64(scenario.CacheWrite)
	}
	// The comparison scenario specifies only standard cache writes. Long TTL
	// variables are valid, but have no usage in this scenario on either side.
	return billingexpr.TokenParams{P: p, C: float64(scenario.OutputTokens), Len: float64(scenario.InputTokens) + float64(scenario.CacheRead) + float64(scenario.CacheWrite), CR: float64(scenario.CacheRead), CC: float64(scenario.CacheWrite), CC1h: 0, CC12h: 0}
}

func monitorUpstreamPrice(price MonitorModelPrice, scenario MonitorScenario) (float64, string, error) {
	if err := validateMonitorScenarioQuantities(scenario); err != nil {
		return 0, "", err
	}
	if price.PricingPending {
		return 0, "", errors.New("Upstream pricing rules need confirmation")
	}
	if err := validateMonitorPriceTiers(price); err != nil {
		return 0, "", err
	}
	if price.BillingExpression != "" || price.BillingUnit == "expression" {
		if len(price.Tiers) != 0 || price.BillingUnit != "expression" {
			return 0, "", errors.New("Invalid upstream billing expression")
		}
		// Captured coefficients retain the validated row/site currency. Reporting
		// conversion is applied by monitorProfitFactors after evaluation.
		cost, trace, err := monitorScenarioExpression(price.BillingExpression, scenario, true)
		return cost, trace.MatchedTier, err
	}
	if price.BillingUnit == "request" {
		if price.RequestPrice == nil {
			return 0, "", errors.New("Upstream request price is unknown")
		}
		cost := *price.RequestPrice * float64(scenario.Requests)
		if !monitorFinite(cost) {
			return 0, "", errors.New("Scenario amount is outside the supported range")
		}
		return cost, "", nil
	}
	if price.BillingUnit != "token" || price.TokenUnit == nil || !monitorFinite(*price.TokenUnit) || *price.TokenUnit <= 0 {
		return 0, "", errors.New("Upstream billing rule or token unit is unsupported")
	}
	if price.Input == nil || price.Output == nil {
		return 0, "", errors.New("A required upstream unit price is unknown")
	}
	rates, label, cost, err := monitorUpstreamTokenPrices(price, scenario)
	if err != nil {
		return 0, "", err
	}
	quantities := [4]int{scenario.InputTokens, scenario.OutputTokens, scenario.CacheRead, scenario.CacheWrite}
	if price.TierMode == "graduated" {
		quantities[0] = 0 // Already charged by bracket.
	}
	for i, count := range quantities {
		if count <= 0 {
			continue
		}
		if rates[i] == nil {
			return 0, "", errors.New("A required upstream unit price is unknown")
		}
		cost += float64(count) / *price.TokenUnit * *rates[i]
	}
	cost *= float64(scenario.Requests)
	if !monitorFinite(cost) {
		return 0, "", errors.New("Scenario amount is outside the supported range")
	}
	return cost, label, nil
}

// Returns prices in the upstream token unit and the cost already attributed to
// graduated input. Its effective input rate describes this scenario only.
func monitorUpstreamTokenPrices(price MonitorModelPrice, scenario MonitorScenario) ([4]*float64, string, float64, error) {
	rates := [4]*float64{price.Input, price.Output, price.CacheRead, price.CacheWrite}
	if len(price.Tiers) == 0 {
		return rates, "", 0, nil
	}
	label := "base"
	quantity := int64(scenario.InputTokens)
	if price.TierBasis == "context" {
		quantity += int64(scenario.CacheRead) + int64(scenario.CacheWrite)
	}
	if price.TierMode == "whole_request" {
		for _, tier := range price.Tiers {
			if quantity <= *tier.InputThreshold {
				break
			}
			rates = [4]*float64{tier.Input, tier.Output, tier.CacheRead, tier.CacheWrite}
			label = fmt.Sprintf("%s > %d", price.TierBasis, *tier.InputThreshold)
		}
		return rates, label, 0, nil
	}
	if quantity == 0 {
		return rates, label, 0, nil
	}
	lower := int64(0)
	inputPrice := price.Input
	cost := 0.0
	for _, tier := range price.Tiers {
		upper := min(quantity, *tier.InputThreshold)
		if inputPrice == nil {
			return rates, label, 0, errors.New("A required upstream unit price is unknown")
		}
		cost += float64(upper-lower) / *price.TokenUnit * *inputPrice
		lower = upper
		if quantity <= *tier.InputThreshold {
			break
		}
		inputPrice = tier.Input
		label = fmt.Sprintf("input > %d", *tier.InputThreshold)
	}
	if quantity > lower {
		if inputPrice == nil {
			return rates, label, 0, errors.New("A required upstream unit price is unknown")
		}
		cost += float64(quantity-lower) / *price.TokenUnit * *inputPrice
	}
	rates[0] = monitorKnownAmount(cost / float64(quantity) * *price.TokenUnit)
	return rates, label, cost, nil
}
