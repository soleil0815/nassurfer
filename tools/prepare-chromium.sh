#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Bereitet Chromium (headless shell) für das Synology-Paket vor:
#   - Chromium aus dem npm-Paket @sparticuz/chromium entpacken
#   - eigene glibc, NSS, sqlite und Schriften beilegen
#   - Chromium und Bibliotheken fest auf den Paketpfad verdrahten (patchelf),
#     damit es unabhängig von den DSM-Systembibliotheken läuft
#
# Aufruf:  tools/prepare-chromium.sh <ausgabe-ordner>
# Braucht: npm, python3 (+ brotli), patchelf, apt-get download (Ubuntu/Debian)
set -eu

OUT=${1:?Ausgabe-Ordner fehlt}
PKG_TARGET=${PKG_TARGET:-/var/packages/NASsurfer/target/chromium}
CHROMIUM_NPM=${CHROMIUM_NPM:-@sparticuz/chromium}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM

echo "Lade $CHROMIUM_NPM …"
(cd "$work" && npm pack --silent "$CHROMIUM_NPM" >/dev/null && tar -xzf ./*.tgz)

python3 - "$work" <<'EOF'
import brotli, sys, os
w = sys.argv[1]
for f in ("chromium", "al2023.tar", "swiftshader.tar"):
    src = os.path.join(w, "package", "bin", f + ".br")
    open(os.path.join(w, f), "wb").write(brotli.decompress(open(src, "rb").read()))
EOF

rm -rf "$OUT"
mkdir -p "$OUT/lib" "$OUT/fonts"
cp "$work/chromium" "$OUT/chromium"
chmod 0755 "$OUT/chromium"
tar -xf "$work/al2023.tar" -C "$work" && cp "$work"/lib/* "$OUT/lib/"
tar -xf "$work/swiftshader.tar" -C "$OUT"

# Schriften: DejaVu (breite Abdeckung) und Liberation (maßgleich zu Arial/Times/Courier)
(cd "$work" && apt-get download -q fonts-dejavu-core fonts-liberation >/dev/null)
for deb in "$work"/fonts-*.deb; do
    mkdir -p "$work/deb" && dpkg-deb -x "$deb" "$work/deb"
done
find "$work/deb" -name '*.ttf' -exec cp {} "$OUT/fonts/" \;

# glibc & Co. aus dem Build-System: alles, was Chromium und die Bibliotheken brauchen
copy_deps() {
    ldd "$1" 2>/dev/null | awk '/=> \// {print $3} /^\t\/lib/ {print $1}' | while read -r lib; do
        base=$(basename "$lib")
        [ -e "$OUT/lib/$base" ] && continue
        cp -L "$lib" "$OUT/lib/$base"
        copy_deps "$lib"
    done
}
copy_deps "$OUT/chromium"
for f in "$OUT"/lib/*.so* "$OUT"/*.so*; do copy_deps "$f"; done
cp -L /lib64/ld-linux-x86-64.so.2 "$OUT/lib/ld-linux-x86-64.so.2"

# Fest verdrahten: eigener Loader, eigene Bibliotheken (DT_RPATH wirkt auch für Abhängigkeiten)
patchelf --set-interpreter "$PKG_TARGET/lib/ld-linux-x86-64.so.2" --force-rpath --set-rpath "$PKG_TARGET/lib:$PKG_TARGET" "$OUT/chromium"
for f in "$OUT"/lib/*.so* "$OUT"/*.so*; do
    case "$(basename "$f")" in ld-linux*) continue ;; esac
    patchelf --force-rpath --set-rpath "$PKG_TARGET/lib:$PKG_TARGET" "$f" 2>/dev/null || true
done

# Lizenztexte der mitgelieferten Fremdsoftware
mkdir -p "$OUT/licenses"
cp "$work/package/LICENSE" "$OUT/licenses/sparticuz-chromium.LICENSE"
for p in libc6 libsqlite3-0 libgcc-s1; do
    [ -f "/usr/share/doc/$p/copyright" ] && cp "/usr/share/doc/$p/copyright" "$OUT/licenses/$p.copyright"
done
for p in fonts-dejavu-core fonts-liberation; do
    f=$(find "$work/deb" -path "*/doc/$p/copyright" | head -1)
    [ -n "$f" ] && cp "$f" "$OUT/licenses/$p.copyright"
done

grep -o '"version": *"[^"]*"' "$work/package/package.json" | head -1 | sed 's/.*"\([^"]*\)"$/\1/' > "$OUT/VERSION"
echo "Chromium $(cat "$OUT/VERSION") bereit in $OUT ($(du -sh "$OUT" | cut -f1))"
