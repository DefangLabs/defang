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

# Configure user-scoped global npm installs (no sudo, works outside nix shell).
mkdir -p "$HOME/.npm-global"
if ! grep -q "npm-global/bin" "$HOME/.bashrc" 2>/dev/null; then
  echo 'export PATH="$HOME/.npm-global/bin:$PATH"' >> "$HOME/.bashrc"
fi
if ! [ -f "$HOME/.npmrc" ] || ! grep -q "^prefix=" "$HOME/.npmrc"; then
  echo "prefix=$HOME/.npm-global" >> "$HOME/.npmrc"
fi
'
