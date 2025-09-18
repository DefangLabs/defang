#!/bin/sh
set -e

if [ "$SNAPSHOT" = "true" ] || [ "$NIGHTLY" = "true" ]; then
  echo "Skipping codesign for snapshot/nightly build"
  exit 0
fi

# Bail if we didn't get one (and only one) argument
if [ $# -ne 1 ]; then
    echo "Usage: $0 <path to app to sign>"
    exit 1
fi

if [ -z "$MACOS_CERTIFICATE_NAME" ]; then
    echo "Error: missing env var MACOS_CERTIFICATE_NAME"
    exit 2
fi

[ "$ACTIONS_STEP_DEBUG" = 'true' ] || [ "$DEBUG" = 'true' ] && set -x

# We need to create a temporary keychain to store our certificate and provisioning profile, but only in CI
if [ -n "$RUNNER_TEMP" ]; then
    # assume MACOS_P12_BASE64, KEYCHAIN_PASSWORD, MACOS_P12_PASSWORD are set in the env

    # create variables with unique names to avoid conflicts
    UNIQUE_ID=$(date +%s)-$$-$(od -An -N4 -tx4 < /dev/urandom | tr -d ' ')
    TMP_CERTIFICATE_PATH=$RUNNER_TEMP/build_certificate_${UNIQUE_ID}.p12
    TMP_KEYCHAIN_PATH=$RUNNER_TEMP/app-signing-${UNIQUE_ID}.keychain-db

    # Clean up any existing certificates in default keychain that might conflict
    security delete-identity -c "$MACOS_CERTIFICATE_NAME" 2>/dev/null || true

    # import certificate and provisioning profile from secrets
    echo $MACOS_P12_BASE64 | base64 --decode > "$TMP_CERTIFICATE_PATH"

    # We need to create a new keychain, otherwise using the certificate will prompt
    # with a UI dialog asking for the certificate password, which we can't
    # use in a headless CI environment

    # Create new keychain (should be unique, but remove if exists just in case)
    security delete-keychain "$TMP_KEYCHAIN_PATH" 2>/dev/null || true
    security create-keychain -p "$KEYCHAIN_PASSWORD" "$TMP_KEYCHAIN_PATH"
    # security set-keychain-settings -lut 21600 "$TMP_KEYCHAIN_PATH"
        # security default-keychain -s "$TMP_KEYCHAIN_PATH"
    security unlock-keychain -p "$KEYCHAIN_PASSWORD" "$TMP_KEYCHAIN_PATH"

    # import certificate to keychain
    security import "$TMP_CERTIFICATE_PATH" -P "$MACOS_P12_PASSWORD" -t cert -f pkcs12 -k "$TMP_KEYCHAIN_PATH" -T /usr/bin/codesign
    security list-keychain -d user -s "$TMP_KEYCHAIN_PATH"

    security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$KEYCHAIN_PASSWORD" "$TMP_KEYCHAIN_PATH"

    # Clean up the certificate file
    rm -f "$TMP_CERTIFICATE_PATH"

    # Set up cleanup trap
    cleanup() {
        echo "Cleaning up keychain..."
        security delete-keychain "$TMP_KEYCHAIN_PATH" 2>/dev/null || true
    }
    trap cleanup EXIT
fi

# We finally codesign our app bundle. Add '--options runtime' for the Hardened runtime option (required for notarization)
codesign --force -s "$MACOS_CERTIFICATE_NAME" "$1" -v --timestamp --options runtime,library
