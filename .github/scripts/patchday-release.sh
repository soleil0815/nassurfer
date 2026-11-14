#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Veröffentlicht den getesteten Patchday-Build als Release (Englisch und Deutsch).
set -eu
SPK="dist/NASsurfer-x86_64-$VERSION.spk"
changes=$(git log --no-merges --format='- %s' "$LAST_TAG..HEAD" 2>/dev/null || true)
[ -n "$changes" ] || changes="- –"
old=$OLD_CHROMIUM
[ "$old" = "unknown" ] && old="?"
if [ "$old" = "$CHROMIUM" ]; then
	chrome_en="Chromium **$CHROMIUM** (unchanged)"
	chrome_de="Chromium **$CHROMIUM** (unverändert)"
else
	chrome_en="Chromium **$CHROMIUM** (previously $old) with the latest security fixes"
	chrome_de="Chromium **$CHROMIUM** (bisher $old) mit den aktuellen Sicherheitskorrekturen"
fi

cat > /tmp/notes.md <<NOTES
## English
**Patchday update** – built and end-to-end tested automatically.

- $chrome_en

**Changes since $LAST_TAG**
$changes

**Install / update:** Package Center → Manual Install → \`NASsurfer-x86_64-$VERSION.spk\`
DSM 7.0–7.2 · x86_64 · License: AGPL-3.0

## Deutsch
**Patchday-Update** – automatisch gebaut und Ende-zu-Ende getestet.

- $chrome_de

**Änderungen seit $LAST_TAG**
$changes

**Installation / Update:** Paket-Zentrum → Manuelle Installation → \`NASsurfer-x86_64-$VERSION.spk\`
DSM 7.0–7.2 · x86_64 · Lizenz: AGPL-3.0

<!-- nassurfer:chromium=$CHROMIUM -->
NOTES

gh release create "v$VERSION" "$SPK" --target "$GITHUB_SHA" --title "NASsurfer $VERSION" --notes-file /tmp/notes.md
echo "Release v$VERSION veröffentlicht"
