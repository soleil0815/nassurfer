#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Vermerkt die Chromium-Version unsichtbar in der Release-Beschreibung.
# Der Patchday-Workflow liest sie, um zu erkennen, ob es einen neueren Chromium gibt.
set -eu
TAG=$1
VER=$2
body=$(gh release view "$TAG" --json body --jq .body)
body=$(printf '%s\n' "$body" | grep -v '<!-- nassurfer:chromium=' || true)
printf '%s\n\n<!-- nassurfer:chromium=%s -->\n' "$body" "$VER" > /tmp/notes.md
gh release edit "$TAG" --notes-file /tmp/notes.md >/dev/null
echo "Chromium $VER an $TAG vermerkt"
