#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Werkzeuge für Bau und Test auf dem GitHub-Runner (Ubuntu).
set -eu
sudo apt-get update -q >/dev/null
python3 -m pip install --user --break-system-packages --quiet brotli patchelf playwright
echo "$HOME/.local/bin" >> "${GITHUB_PATH:-/dev/null}"
export PATH="$HOME/.local/bin:$PATH"
patchelf --version
command -v google-chrome
go version
