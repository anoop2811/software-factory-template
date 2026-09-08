package usage

import (
	"context"
	"math/big"
)

type stepSignature struct {
	tokens, cache map[string]*big.Int
	cost, reason  any
}

// Deduplicate before admission and keep integer aggregates independent of cost.
// docs/adr/0064-go-native-usage-accounting.md:56.
func opencode(ctx context.Context, events []any) Metadata {
	result := Metadata{Source: "opencode.step_finish.part (deduplicated incremental client estimate)"}
	seen := make(map[[3]string]stepSignature)
	totals := map[string]*big.Int{
		"input_tokens": new(big.Int), "output_tokens": new(big.Int),
		"reasoning_output_tokens": new(big.Int), "cache_read_input_tokens": new(big.Int),
		"cache_creation_input_tokens": new(big.Int),
	}
	totalCost := 0.0
	var lastReason any
	valid := true
	for _, event := range events {
		if ctx.Err() != nil {
			return result
		}
		row := object(event)
		if row == nil {
			valid = false
			continue
		}
		switch text(row["type"]) {
		case "error", "session.error":
			result.Failed = true
		}
		if text(row["type"]) != "step_finish" {
			continue
		}
		part := object(row["part"])
		identity := [3]string{text(part["sessionID"]), text(part["messageID"]), text(part["id"])}
		if part == nil || identity[0] == "" || identity[1] == "" || identity[2] == "" {
			valid = false
			continue
		}
		values := object(part["tokens"])
		signature := stepSignature{
			tokens: tokens(values, []string{"input", "output", "reasoning"}, "total"),
			cache:  tokens(values["cache"], []string{"read", "write"}),
			cost:   part["cost"], reason: part["reason"],
		}
		if previous, exists := seen[identity]; exists {
			if !equalTokens(previous.tokens, signature.tokens) || !equalTokens(previous.cache, signature.cache) ||
				!equalValue(ctx, previous.cost, signature.cost) || !equalValue(ctx, previous.reason, signature.reason) {
				valid = false
			}
			continue
		}
		seen[identity] = signature
		lastReason = signature.reason
		_, cost, costValid := finiteNumber(signature.cost)
		if signature.tokens == nil || signature.cache == nil || !costValid {
			valid = false
			continue
		}
		for _, pair := range [][2]string{
			{"input_tokens", "input"}, {"output_tokens", "output"}, {"reasoning_output_tokens", "reasoning"},
		} {
			totals[pair[0]].Add(totals[pair[0]], signature.tokens[pair[1]])
		}
		totals["cache_read_input_tokens"].Add(totals["cache_read_input_tokens"], signature.cache["read"])
		totals["cache_creation_input_tokens"].Add(totals["cache_creation_input_tokens"], signature.cache["write"])
		totalCost += cost
	}
	if len(seen) > 0 && valid && !result.Failed && text(lastReason) == "stop" && finite(totalCost) {
		cost := floatNumber(totalCost)
		result.Tokens = totals
		result.EstimatedUSD = &cost
		result.Complete = true
	}
	return result
}
