#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
image='jxxghp/moviepilot-v3:3.1.4'
log=$(mktemp)
trap 'rm -f "$log"' EXIT
docker run --rm --network none \
  -e CONFIG_DIR=/tmp/crychic-fixture -e MOVIEPILOT_SAFE_MODE=true \
  -e API_TOKEN=crychic-fixture-api-key -e SUPERUSER=fixture-admin -e LOG_LEVEL=ERROR \
  --mount "type=bind,source=$PWD/plugins.v3/crychic,target=/app/app/plugins/crychic,readonly" \
  --mount "type=bind,source=$PWD/e2e/mp_plugin_host.py,target=/fixture.py,readonly" \
  --entrypoint python "$image" /fixture.py >"$log" 2>&1 || {
    tail -30 "$log"
    exit 1
  }
grep '^CRYCHIC_PLUGIN_E2E ' "$log" | sed 's/^CRYCHIC_PLUGIN_E2E //' >"$log.trace"
trap 'rm -f "$log" "$log.trace"' EXIT
if [[ ${1:-} == --update ]]; then
  cp "$log.trace" e2e/testdata/transcripts/MPPluginNativeHost.txt
fi
diff -u e2e/testdata/transcripts/MPPluginNativeHost.txt "$log.trace"
printf 'MP native plugin E2E passed\n'
