#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Ende-zu-Ende-Test eines fertigen Pakets:
#   - .spk wie auf DSM nach /var/packages/NASsurfer installieren
#   - Dienst mit dem DSM-Startskript starten
#   - nachgebautes DSM + Test-Gerät starten, Tests im Browser ausführen
#   - Dienst stoppen und prüfen, dass kein Passwort im Profil steht
#
# Aufruf: test/e2e/run.sh dist/NASsurfer-x86_64-<version>.spk
# Braucht: root-Rechte für /var/packages (oder sudo), go, python3 + playwright, Google Chrome
set -eu

SPK=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
PKG=/var/packages/NASsurfer
SUDO=""
[ "$(id -u)" -eq 0 ] || SUDO="sudo"

work=$(mktemp -d)
harness_pid=""
cleanup() {
    [ -n "$harness_pid" ] && kill "$harness_pid" 2>/dev/null || true
    SYNOPKG_PKGVAR="$PKG/var" sh "$work/spk/scripts/start-stop-status" stop >/dev/null 2>&1 || true
    rm -rf "$work"
}
trap cleanup EXIT INT TERM

echo "Installiere $(basename "$SPK") …"
mkdir -p "$work/spk"
tar -xf "$SPK" -C "$work/spk"
$SUDO rm -rf "$PKG"
$SUDO mkdir -p "$PKG/target" "$PKG/var"
$SUDO chown -R "$(id -u):$(id -g)" "$PKG"
tar -xzf "$work/spk/package.tgz" -C "$PKG/target"
grep '^version=' "$work/spk/INFO"

echo "Starte Dienst …"
SYNOPKG_PKGVAR="$PKG/var" sh "$work/spk/scripts/start-stop-status" start
SYNOPKG_PKGVAR="$PKG/var" sh "$work/spk/scripts/start-stop-status" status

echo "Starte Testaufbau …"
(cd "$ROOT/test/e2e/harness" && go build -o "$work/harness" .)
"$work/harness" -cgi "$PKG/target/ui/browser.cgi" -uisrc "$ROOT/package/ui/src" > "$work/harness.log" 2>&1 &
harness_pid=$!
sleep 1

rc=0
python3 "$ROOT/test/e2e/e2e.py" --var "$PKG/var" || rc=$?

echo "Stoppe Dienst …"
SYNOPKG_PKGVAR="$PKG/var" sh "$work/spk/scripts/start-stop-status" stop
if pgrep -f "$PKG/target/chromium/chromium" >/dev/null 2>&1; then
    echo "  ✘ Chromium läuft nach dem Stoppen noch"
    rc=1
else
    echo "  ✔ Dienst und Chromium sauber beendet"
fi
python3 "$ROOT/test/e2e/e2e.py" --after-stop --var "$PKG/var" || rc=$?

if [ "$rc" -ne 0 ]; then
    echo "--- daemon.log (Ende) ---"
    tail -40 "$PKG/var/daemon.log" 2>/dev/null || true
fi
exit "$rc"
