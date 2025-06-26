#!/bin/sh
set -e

if [ "$SNAPSHOT" = "true" ] || [ "$NIGHTLY" = "true" ]; then
  echo "Skipping notarization for snapshot/nightly build"
  exit 0
fi

# Bail if we didn't get one (and only one) argument
if [ $# -ne 1 ]; then
    echo "Usage: $0 <path to app to notarize>"
    exit 1
fi

if [ -z "$MACOS_NOTARIZATION_APPLE_ID" ]; then
    echo "Error: missing env var MACOS_NOTARIZATION_APPLE_ID"
    exit 2
fi

if [ -z "$MACOS_NOTARIZATION_TEAM_ID" ]; then
    echo "Error: missing env var MACOS_NOTARIZATION_TEAM_ID"
    exit 3
fi

if [ -z "$MACOS_NOTARIZATION_APP_PW" ]; then
    echo "Error: missing env var MACOS_NOTARIZATION_APP_PW"
    exit 4
fi

[ "$ACTIONS_STEP_DEBUG" = 'true' ] || [ "$DEBUG" = 'true' ] && set -x

xcrun notarytool submit "$1" --apple-id "$MACOS_NOTARIZATION_APPLE_ID" --team-id "$MACOS_NOTARIZATION_TEAM_ID" --password "$MACOS_NOTARIZATION_APP_PW"
