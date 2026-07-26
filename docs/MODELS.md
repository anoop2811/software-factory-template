# Models and providers

The factory does not assume you use any particular model provider, or that you
have keys for one. Model values are plain strings in `factory.config`, and blank
means **inherit** — the factory writes no model pins at all and each harness keeps
whatever it is already configured with.

## Choosing at init

`factory-init` asks for a provider once. It only *seeds* defaults:

| Provider | What gets seeded |
|---|---|
| `inherit` | Nothing. Every tier stays blank; each harness keeps its own model. Model pins are removed from `opencode.json` so nothing is overridden. |
| `openrouter` | opencode tiers as OpenRouter slugs, plus native Claude/Codex tiers |
| `anthropic` | opencode tiers as `anthropic/…`, plus native Claude/Codex tiers |
| `openai` | opencode tiers as `openai/…`, plus native Claude/Codex tiers |
| anything else | opencode tiers blank (set them yourself), plus native Claude/Codex tiers |

Nothing here is locked in — edit `factory.config` and run `make sync-harnesses`
(Decision 25) and every harness re-routes.

## Why the three harnesses differ

opencode reaches [75+ providers](https://opencode.ai/docs/providers/) with a
`provider/model` string, so its tiers follow whichever provider you chose. Claude
Code only talks to Anthropic and Codex only to OpenAI, so their tiers always use
those native ids — and they only apply if you actually run that harness.

```sh
# factory.config — the shape, whatever the provider
MODEL_PROVIDER="openrouter"
OPENCODE_FRONTIER_MODEL="openrouter/z-ai/glm-5.2"
OPENCODE_DEFAULT_MODEL="openrouter/z-ai/glm-5.2"
OPENCODE_ECONOMY_MODEL="openrouter/qwen/qwen3-coder"
CLAUDE_FRONTIER_MODEL="claude-opus-4-8"      # Claude Code: Anthropic ids
CODEX_FRONTIER_MODEL="gpt-5.6-sol"           # Codex: OpenAI ids
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

The factory never reads or stores a key. Credentials belong to your harness (and,
if you enable a CI lane that calls a model, to your CI secrets) — see
[opencode's provider docs](https://opencode.ai/docs/providers/) for how each one
is authenticated.

## Tiers, not models

Which model runs where is decided by *role tier*, not by the model string — see
[COST_AND_TOKENS.md](COST_AND_TOKENS.md). `spec-writer` and `reviewer` are
frontier, `refactorer` and `wiki-maintainer` are economy, everything else is
default; the `economy` cost profile turns the cheap tier on. With
`MODEL_PROVIDER="inherit"` the tiers are blank, so the cost profile routes
nothing until you fill them in.
