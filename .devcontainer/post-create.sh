#!/usr/bin/env bash
set -euo pipefail

# Expose Go tooling to VS Code by symlinking nix-provided binaries, then run setup.
nix develop --command bash -lc '
set -euo pipefail
sudo ln -sf "$(which go)" /usr/local/bin/go
if command -v gopls >/dev/null; then
  sudo ln -sf "$(which gopls)" /usr/local/bin/gopls
fi
make setup
'
