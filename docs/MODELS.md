# Models and providers

The factory does not assume you use any particular model provider, or that you
have keys for one. Model values are plain strings in `factory.yaml`, and blank
means **inherit** — the factory writes no model pins at all and each harness keeps
whatever it is already configured with.

## Choosing at init

The Go initializer asks for a provider during project setup. It only *seeds* defaults:

| Provider | What gets seeded |
|---|---|
| `inherit` | Nothing. Every tier stays blank; each harness keeps its own model. Model pins are removed from `opencode.json` so nothing is overridden. |
| `openrouter` | opencode tiers as OpenRouter slugs, plus native Claude/Codex tiers |
| `anthropic` | opencode tiers as `anthropic/…`, plus native Claude/Codex tiers |
| `openai` | opencode tiers as `openai/…`, plus native Claude/Codex tiers |
| anything else | opencode tiers blank (set them yourself), plus native Claude/Codex tiers |

To change the selection, edit `factory.yaml`, run `./factory sync all` and
inspect the generated diff.

## Why the three harnesses differ

The factory writes OpenCode model IDs as provider/model strings. It seeds
Claude Code and Codex with identifiers in their own native formats. The
identifiers below are v0.2.0 configuration examples. Availability and billing
depend on the provider and account you actually use.

```yaml
# factory.yaml — the shape, whatever the provider
model_provider: "openrouter"
opencode_frontier_model: "openrouter/z-ai/glm-5.2"
opencode_default_model: "openrouter/z-ai/glm-5.2"
opencode_economy_model: "openrouter/qwen/qwen3-coder"
claude_frontier_model: "claude-opus-4-8"     # Claude Code: Anthropic ids
codex_frontier_model: "gpt-5.6-sol"          # Codex: OpenAI ids
```

## Example model strings

Treat these as starting points and **verify them against your provider's current
model list and your own plan** — model names move, and availability differs by
account.

| Provider | opencode string | Credential |
|---|---|---|
| OpenRouter | `openrouter/z-ai/glm-5.2` | `OPENROUTER_API_KEY` |
| Anthropic | `anthropic/claude-sonnet-4-6` | `ANTHROPIC_API_KEY` |
| OpenAI | `openai/gpt-5.6-terra` | `OPENAI_API_KEY` |
| Ollama (local) | `ollama/<your-tag>` | none |
| Bedrock / Vertex / Azure | provider-prefixed, per opencode's provider docs | that cloud's credentials |

Use your harness's credential settings for interactive work. If you enable
the optional advisory PR review, the factory review client reads the API key
from its environment. Configure the corresponding Actions secret and put only
its **name** in `factory.yaml`. [Review lane setup](REVIEW_LANE.md).

## Tiers, not models

Which model runs where is decided by *role tier*, not by the model string — see
[COST_AND_TOKENS.md](COST_AND_TOKENS.md). `spec-writer` and `reviewer` are
frontier, `refactorer` and `wiki-maintainer` are economy, everything else is
default; the `economy` cost profile turns the cheap tier on. With
`model_provider: "inherit"` the tiers are blank, so the cost profile routes
nothing until you fill them in.
