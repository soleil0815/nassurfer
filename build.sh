#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Baut das Synology-Paket (.spk) für NASsurfer.
#   ./build.sh                       → dist/NASsurfer-x86_64-<version>.spk
#   VERSION=1.0.0-0008 ./build.sh
#   CHROMIUM_DIR=/pfad ./build.sh    → bereits vorbereiteten Chromium verwenden
# Benötigt: go, tar, gzip, md5sum (oder md5); für Chromium siehe tools/prepare-chromium.sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
VERSION=${VERSION:-1.0.0-0003}
ARCH=x86_64 # Chromium gibt es hier nur für Intel/AMD
DIST="$ROOT/dist"
CHROMIUM_DIR=${CHROMIUM_DIR:-"$ROOT/build/chromium"}

md5_of() {
    if command -v md5sum >/dev/null 2>&1; then
        md5sum "$1" | cut -d' ' -f1
    else
        md5 -q "$1"
    fi
}

if tar --version 2>/dev/null | grep -q 'GNU tar'; then
    TAR_OWNER="--owner=0 --group=0 --numeric-owner"
else
    TAR_OWNER="--uid 0 --gid 0"
fi

echo "Tests …"
(cd "$ROOT/cgi" && go vet ./... && go test ./...)

if [ ! -x "$CHROMIUM_DIR/chromium" ]; then
    "$ROOT/tools/prepare-chromium.sh" "$CHROMIUM_DIR"
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM
mkdir -p "$work/pkg/bin" "$work/pkg/ui" "$work/spk"

echo "Baue Programm …"
(cd "$ROOT/cgi" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o "$work/pkg/bin/nassurfer" .)
cp "$work/pkg/bin/nassurfer" "$work/pkg/ui/browser.cgi"

# Oberfläche: Browser-Kern + DSM-Fenster zu einer Datei
for f in config start.html; do cp "$ROOT/package/ui/$f" "$work/pkg/ui/"; done
cp -R "$ROOT/package/ui/images" "$ROOT/package/ui/texts" "$work/pkg/ui/"
cat "$ROOT/package/ui/src/client.js" "$ROOT/package/ui/src/window.js" > "$work/pkg/ui/app.js"

cp -R "$CHROMIUM_DIR" "$work/pkg/chromium"
cp "$ROOT/LICENSE" "$ROOT/THIRD_PARTY_NOTICES.md" "$work/pkg/"

find "$work/pkg" -type d -exec chmod 0755 {} +
find "$work/pkg/ui" "$work/pkg/chromium/fonts" "$work/pkg/chromium/licenses" -type f -exec chmod 0644 {} +
chmod 0644 "$work/pkg/LICENSE" "$work/pkg/THIRD_PARTY_NOTICES.md"
chmod 0755 "$work/pkg/bin/nassurfer" "$work/pkg/ui/browser.cgi" "$work/pkg/chromium/chromium"

echo "Packe …"
# shellcheck disable=SC2086
tar $TAR_OWNER -C "$work/pkg" -czf "$work/spk/package.tgz" bin ui chromium LICENSE THIRD_PARTY_NOTICES.md

sum=$(md5_of "$work/spk/package.tgz")
sed -e "s/@VERSION@/$VERSION/" -e "s/@ARCH@/$ARCH/" -e "s/@CHECKSUM@/$sum/" \
    "$ROOT/package/INFO.in" > "$work/spk/INFO"
cp -R "$ROOT/package/scripts" "$ROOT/package/conf" "$work/spk/"
cp "$ROOT/package/PACKAGE_ICON.PNG" "$ROOT/package/PACKAGE_ICON_256.PNG" "$work/spk/"
chmod 0755 "$work/spk/scripts/"*
chmod 0644 "$work/spk/conf/"* "$work/spk/INFO" "$work/spk/PACKAGE_ICON.PNG" "$work/spk/PACKAGE_ICON_256.PNG"

mkdir -p "$DIST"
out="$DIST/NASsurfer-$ARCH-$VERSION.spk"
# shellcheck disable=SC2086
tar $TAR_OWNER -C "$work/spk" -cf "$out" INFO package.tgz scripts conf PACKAGE_ICON.PNG PACKAGE_ICON_256.PNG
echo "  → $out ($(du -h "$out" | cut -f1)), Chromium $(cat "$CHROMIUM_DIR/VERSION" 2>/dev/null || echo ?)"
