#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Erzeugt die README-Bilder (docs/images/de und docs/images/en) mit dem echten
# Paket: .spk installieren, Dienst starten, nachgestelltes DSM und ausgedachte
# Geräte bereitstellen, Bilder im Browser aufnehmen.
#
# Aufruf (als root, z. B. in einem Wegwerf-Container):
#   tools/screenshots/run.sh dist/NASsurfer-x86_64-<version>.spk
# Trägt diskstation, router.lan, drucker.lan, container.lan in /etc/hosts ein.
# Braucht: go, python3 + playwright + Pillow, Google Chrome
set -eu

[ "$(id -u)" -eq 0 ] || { echo "Bitte als root ausführen (Port 80, /etc/hosts, /var/packages)." >&2; exit 1; }
SPK=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
PKG=/var/packages/NASsurfer

# Interne Adressen dürfen nicht über einen Proxy laufen
unset HTTPS_PROXY HTTP_PROXY https_proxy http_proxy ALL_PROXY all_proxy

work=$(mktemp -d)
server_pid=""
cleanup() {
    [ -n "$server_pid" ] && kill "$server_pid" 2>/dev/null || true
    SYNOPKG_PKGVAR="$PKG/var" sh "$work/spk/scripts/start-stop-status" stop >/dev/null 2>&1 || true
    rm -rf "$work"
}
trap cleanup EXIT INT TERM

grep -q "nassurfer-screenshots" /etc/hosts ||
    echo "127.0.0.1 diskstation router.lan drucker.lan container.lan # nassurfer-screenshots" >> /etc/hosts

mkdir -p "$work/spk"
tar -xf "$SPK" -C "$work/spk"
rm -rf "$PKG"
mkdir -p "$PKG/target" "$PKG/var"
tar -xzf "$work/spk/package.tgz" -C "$PKG/target"
SYNOPKG_PKGVAR="$PKG/var" sh "$work/spk/scripts/start-stop-status" start

(cd "$ROOT/tools/screenshots/server" && go build -o "$work/server" .)
"$work/server" -root "$ROOT" -cgi "$PKG/target/ui/browser.cgi" > "$work/server.log" 2>&1 &
server_pid=$!
sleep 1

python3 "$ROOT/tools/screenshots/shots.py" --out "$ROOT/docs/images"
