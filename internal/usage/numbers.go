package usage

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
)

func integerSyntax(number json.Number) bool {
	return !strings.ContainsAny(string(number), ".eE")
}

func finite(value float64) bool {
	return !math.IsInf(value, 0) && !math.IsNaN(value)
}

// A finite float conversion cannot admit an integer with more than 309 digits.
// Keep admitted counters exact: docs/adr/0064-go-native-usage-accounting.md:43.
func integer(number json.Number) *big.Int {
	if !integerSyntax(number) || len(strings.TrimPrefix(string(number), "-")) > 309 {
		return nil
	}
	value, ok := new(big.Int).SetString(string(number), 10)
	if !ok {
		return nil
	}
	return value
}

func tokens(value any, required []string, optional ...string) map[string]*big.Int {
	fields := object(value)
	if fields == nil {
		return nil
	}
	keys := append([]string(nil), required...)
	for _, key := range optional {
		if _, exists := fields[key]; exists {
			keys = append(keys, key)
		}
	}
	result := make(map[string]*big.Int, len(keys))
	for _, key := range keys {
		number, ok := fields[key].(json.Number)
		if !ok {
			return nil
		}
		value := integer(number)
		if value == nil || value.Sign() < 0 {
			return nil
		}
		converted, _ := value.Float64()
		if !finite(converted) {
			return nil
		}
		result[key] = value
	}
	return result
}

func finiteNumber(value any) (json.Number, float64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return "", 0, false
	}
	if integerSyntax(number) {
		integer := integer(number)
		if integer == nil || integer.Sign() < 0 {
			return "", 0, false
		}
		converted, _ := integer.Float64()
		return json.Number(integer.String()), converted, finite(converted)
	}
	converted, err := number.Float64()
	if err != nil || !finite(converted) || converted < 0 {
		return "", 0, false
	}
	return floatNumber(converted), converted, true
}

func floatNumber(value float64) json.Number {
	return json.Number(strconv.FormatFloat(value, 'g', -1, 64))
}
