#!/bin/sh
# Generate all shell completions for defang
# Usage: ./completions.sh [path/to/defang]

set -e # Exit on error

OUTPUT_DIR=$PWD
SCRIPT_DIR=`dirname "$0"`
DEFANG=$1

# If no path to the defang CLI is provided, use Go to build and run it
if [ -z "$DEFANG" ]; then
  DEFANG="go run ./cmd/cli/main.go"
  cd "$SCRIPT_DIR/.."
fi

for sh in bash zsh fish powershell; do
  $DEFANG completion "$sh" >"$OUTPUT_DIR/defang.$sh"
done
