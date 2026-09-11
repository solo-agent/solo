#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
for profile in '../invalid' "unpaired-service-profile-test-$$"; do
  output="$(SOLO_DAEMON_PROFILE="$profile" bash "$root/scripts/start-services.sh" 2>&1)" && {
    echo "ERROR: invalid or unpaired profile started services" >&2
    exit 1
  }
  [[ "$output" == *'ERROR: invalid SOLO_DAEMON_PROFILE'* || "$output" == *'ERROR: selected Daemon profile is not paired'* ]]
done
echo 'Service profile validation passed; no services started.'
