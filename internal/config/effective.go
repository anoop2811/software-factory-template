package config

import (
	"context"
	"maps"
)

// Effective composes known configuration exports without overwriting caller
// values, including explicitly empty values. docs/adr/0099-native-factory-orchestration.md:132.
func Effective(ctx context.Context, path string, environment map[string]string) (map[string]string, error) {
	actions, err := ExportPlan(ctx, path, preservedKeys(environment))
	if err != nil {
		return nil, err
	}
	return applyEffective(environment, actions), nil
}

// EffectiveBytes applies the same precedence to already captured inputs.
// docs/adr/0099-native-factory-orchestration.md:132.
func EffectiveBytes(ctx context.Context, yaml, legacy []byte, environment map[string]string) (map[string]string, error) {
	actions, err := ExportPlanBytes(ctx, yaml, legacy, preservedKeys(environment))
	if err != nil {
		return nil, err
	}
	return applyEffective(environment, actions), nil
}

func preservedKeys(environment map[string]string) []string {
	var keys []string
	for key := range environment {
		if KnownExportKey(key) {
			keys = append(keys, key)
		}
	}
	return keys
}

func applyEffective(environment map[string]string, actions []Action) map[string]string {
	effective := maps.Clone(environment)
	if effective == nil {
		effective = map[string]string{}
	}
	for _, item := range actions {
		if !item.ExportOnly {
			effective[item.Key] = item.Value
		}
	}
	return effective
}
