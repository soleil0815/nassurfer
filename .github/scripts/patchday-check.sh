#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Patchday-Prüfung: Ist heute Patchday, und gibt es einen neueren Chromium?
# Ausgabe für $GITHUB_OUTPUT: release, version, chromium, old_chromium, last_tag
set -eu
if [ "${EVENT:-}" = "schedule" ] && [ "$(date -u +%u)" != "2" ]; then
    echo "Heute ist nicht der zweite Dienstag – nichts zu tun." >&2
    echo "release=false"
    exit 0
fi

latest=$(npm view --no-update-notifier @sparticuz/chromium version 2>/dev/null)
last_tag=$(git tag -l 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+-[0-9]{4}$' | sort -V | tail -1)
if [ -z "$last_tag" ]; then
    echo "Kein bisheriger Release gefunden." >&2
    echo "release=false"
    exit 0
fi
old=$(gh release view "$last_tag" --json body --jq .body 2>/dev/null \
    | sed -n 's/.*<!-- nassurfer:chromium=\([0-9.]*\) -->.*/\1/p' | tail -1)
echo "Letzter Release $last_tag mit Chromium ${old:-unbekannt}, verfügbar: $latest" >&2

if [ "$old" = "$latest" ] && [ "${FORCE:-false}" != "true" ]; then
    echo "Chromium ist aktuell – kein neuer Release nötig." >&2
    echo "release=false"
    exit 0
fi

base=${last_tag#v}
build=${base##*-}
base=${base%-*}
next=$(printf '%s-%04d' "$base" "$(( $(echo "$build" | sed 's/^0*//;s/^$/0/') + 1 ))")
echo "Neuer Release: $next" >&2
echo "release=true"
echo "version=$next"
echo "chromium=$latest"
echo "old_chromium=${old:-unknown}"
echo "last_tag=$last_tag"
