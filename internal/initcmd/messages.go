package initcmd

import (
	"context"
	"io"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/output"
)

func detectStack(ctx context.Context, target *tree, out io.Writer) error {
	if target == nil {
		return nil
	}
	var detected []string
	var frameworks []string
	for _, candidate := range [][2]string{{"go.mod", "go"}, {"package.json", "typescript"}, {"pom.xml", "java"}, {"build.gradle", "java"}, {"build.gradle.kts", "java"}} {
		data, _, exists, err := target.read(ctx, candidate[0])
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if !hasPack(detected, candidate[1]) {
			detected = append(detected, candidate[1])
		}
		if candidate[0] == "package.json" {
			if strings.Contains(string(data), `"react"`) {
				frameworks = append(frameworks, "  React detected      → --pack typescript (Biome's react rules auto-apply)\n")
			}
			if strings.Contains(string(data), `"vue"`) {
				frameworks = append(frameworks, "  Vue detected        → --pack typescript (Biome's vue rules auto-apply)\n")
			}
		}
		if candidate[1] == "java" && strings.Contains(strings.ToLower(string(data)), "spring-boot") {
			line := "  Spring Boot detected → --pack java (JUnit 5 + Testcontainers stack)\n"
			if !hasPack(frameworks, line) {
				frameworks = append(frameworks, line)
			}
		}
	}
	if len(detected) == 0 {
		return nil
	}
	return output.WriteEvent(ctx, out, []byte("Detected stack(s): "+strings.Join(detected, " ")+" — install the matching packs/ after init\n"+strings.Join(frameworks, "")))
}
func choiceMessages(ctx context.Context, out io.Writer, v map[string]string, o options) error {
	var text strings.Builder
	if hasPack(o.packs, "java") {
		text.WriteString("Java build tool: " + o.javaBuild + "\n")
	}
	if v["MODEL_PROVIDER"] == "inherit" && v["COST_PROFILE"] == "economy" {
		text.WriteString("  (economy profile selected with provider 'inherit' — set the model tiers\n   in factory.yaml for it to route anything; see docs/MODELS.md)\n")
	}
	if v["MODEL_PROVIDER"] != "inherit" && v["MODEL_PROVIDER"] != "openrouter" && v["MODEL_PROVIDER"] != "anthropic" && v["MODEL_PROVIDER"] != "openai" {
		text.WriteString("  (set opencode_*_model in factory.yaml — see docs/MODELS.md)\n")
	}
	return output.WriteEvent(ctx, out, []byte(text.String()))
}
func packMessages(ctx context.Context, out io.Writer, o options, v map[string]string) error {
	var text strings.Builder
	for _, pack := range o.packs {
		text.WriteString("Installing '" + pack + "' pack...\n")
		maturity := v["PACK_MATURITY_"+pack]
		if maturity == "experimental" {
			text.WriteString("  NOTE: '" + pack + "' is experimental — the full stack ships, but no real repository has adopted it yet.\n")
		}
		if maturity == "beta" {
			text.WriteString("  NOTE: '" + pack + "' is beta — real adoption reported; see the pack documentation for tested scope.\n")
		}
		if pack == "java" && o.javaBuild == "maven" {
			text.WriteString("  Next: merge quality-maven.xml plugins into your parent POM; see MAVEN.md.\n  Until then, verify runs your existing Maven lifecycle, without the new quality plugins.\n")
		}

	}
	return output.WriteEvent(ctx, out, []byte(text.String()))
}
