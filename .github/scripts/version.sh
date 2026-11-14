#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Paketversion bestimmen: bei einem Release aus dem Tag (v1.0.0-0004 → 1.0.0-0004),
# sonst eine Probeversion 0.0.0-<Laufnummer>.
set -eu
if [ "${EVENT:-}" = "release" ]; then
    v=${TAG#v}
    if ! printf '%s\n' "$v" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+-[0-9]{4}$'; then
        echo "Tag '$TAG' hat nicht das Format v1.2.3-0004" >&2
        exit 1
    fi
else
    v=$(printf '0.0.0-%04d' "$(( ${RUN:-1} % 10000 ))")
fi
echo "version=$v"
