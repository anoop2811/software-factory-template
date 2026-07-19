#!/bin/sh
# scripts/lib/roles.sh
# Maps a factory agent role to its model tier: frontier | default | economy.
#
# The tier is a property of the ROLE, not of the model string. opencode's
# frontier and default tiers can share one model (e.g. both GLM 5.2), so a
# generated harness config cannot recover a role's tier by looking at the
# substituted model. sync-claude / sync-codex resolve the tier here instead,
# then read that tier's model for their harness from factory.config.
#
# This mapping must stay in step with the __*_MODEL__ placeholders in the
# canonical opencode.json and .opencode/agent/*.md role files:
#   frontier -> spec-writer, reviewer   (high stakes, low volume)
#   economy  -> refactorer, wiki-maintainer   (low stakes, high volume)
#   default  -> everything else (implementer, and any future role)
role_tier() {
  case "$1" in
    spec-writer|reviewer) printf 'frontier' ;;
    refactorer|wiki-maintainer) printf 'economy' ;;
    *) printf 'default' ;;
  esac
}
