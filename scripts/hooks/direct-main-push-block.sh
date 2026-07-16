#!/bin/bash
set -euo pipefail

# Reads the standard Git pre-push ref update stream and rejects every direct
# update to main. An empty stream is valid when this check is run manually.

while read -r _local_ref _local_sha remote_ref _remote_sha; do
  if [ "$remote_ref" = "refs/heads/main" ]; then
    echo "direct-main-push-block: DENY direct push to main"
    echo "Push a feature branch and open a pull request instead."
    exit 1
  fi
done

echo "direct-main-push-block: no main update detected"
