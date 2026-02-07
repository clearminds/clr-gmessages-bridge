#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="$SCRIPT_DIR/build"
APP_NAME="OpenMessages"
APP_BUNDLE="$BUILD_DIR/$APP_NAME.app"
DMG_PATH="$BUILD_DIR/$APP_NAME.dmg"
ENTITLEMENTS="$SCRIPT_DIR/OpenMessages.entitlements"

# ── Notarization config ──
# Set these env vars to enable code signing + notarization:
#   DEVELOPER_ID   - e.g. "Developer ID Application: Max Ghenis (TEAMID)"
#   APPLE_ID       - e.g. "mghenis@gmail.com"
#   APPLE_TEAM_ID  - e.g. "ABC123XYZ"
#   APP_PASSWORD    - app-specific password from appleid.apple.com
SIGN_IDENTITY="${DEVELOPER_ID:-}"

echo "==> Building Go backend..."
cd "$ROOT_DIR"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o "$SCRIPT_DIR/build/openmessages-arm64" .
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o "$SCRIPT_DIR/build/openmessages-amd64" .
lipo -create -output "$SCRIPT_DIR/build/openmessages" \
    "$SCRIPT_DIR/build/openmessages-arm64" \
    "$SCRIPT_DIR/build/openmessages-amd64"
echo "   Universal binary: $(du -h "$SCRIPT_DIR/build/openmessages" | cut -f1)"

echo "==> Building Swift app..."
cd "$SCRIPT_DIR/OpenMessages"
swift build -c release --arch arm64 --arch x86_64 2>&1 | tail -5

# Find the built executable
SWIFT_BIN=$(swift build -c release --arch arm64 --arch x86_64 --show-bin-path 2>/dev/null)/"$APP_NAME"
if [ ! -f "$SWIFT_BIN" ]; then
    echo "ERROR: Swift binary not found at $SWIFT_BIN"
    echo "Searching..."
    find .build -name "$APP_NAME" -type f 2>/dev/null
    exit 1
fi

echo "==> Assembling $APP_NAME.app..."
rm -rf "$APP_BUNDLE"
mkdir -p "$APP_BUNDLE/Contents/MacOS"
mkdir -p "$APP_BUNDLE/Contents/Resources"

# Copy Swift executable
cp "$SWIFT_BIN" "$APP_BUNDLE/Contents/MacOS/$APP_NAME"

# Copy Go backend binary into Resources
cp "$SCRIPT_DIR/build/openmessages" "$APP_BUNDLE/Contents/Resources/openmessages"
chmod +x "$APP_BUNDLE/Contents/Resources/openmessages"

# Copy Info.plist
cp "$SCRIPT_DIR/OpenMessages/Sources/Info.plist" "$APP_BUNDLE/Contents/Info.plist"

# Create PkgInfo
echo -n "APPL????" > "$APP_BUNDLE/Contents/PkgInfo"

# ── Code signing ──
echo "==> Code signing..."
if [ -n "$SIGN_IDENTITY" ]; then
    echo "   Signing with: $SIGN_IDENTITY"
    # Sign the Go binary first
    codesign --force --options runtime \
        --entitlements "$ENTITLEMENTS" \
        --sign "$SIGN_IDENTITY" \
        "$APP_BUNDLE/Contents/Resources/openmessages"
    # Sign the main app
    codesign --force --options runtime \
        --entitlements "$ENTITLEMENTS" \
        --sign "$SIGN_IDENTITY" \
        "$APP_BUNDLE"
else
    echo "   No DEVELOPER_ID set — using ad-hoc signature"
    codesign --force --deep --sign - "$APP_BUNDLE"
fi

# Remove quarantine attribute
xattr -cr "$APP_BUNDLE"

echo "==> Built: $APP_BUNDLE"
echo "   Size: $(du -sh "$APP_BUNDLE" | cut -f1)"

# ── Create DMG ──
echo "==> Creating DMG..."
rm -f "$DMG_PATH"
hdiutil create -volname "$APP_NAME" -srcfolder "$APP_BUNDLE" -ov -format UDZO "$DMG_PATH" 2>&1 | tail -1
echo "   DMG: $(du -h "$DMG_PATH" | cut -f1)"

# ── Notarize ──
if [ -n "$SIGN_IDENTITY" ] && [ -n "${APPLE_ID:-}" ] && [ -n "${APPLE_TEAM_ID:-}" ] && [ -n "${APP_PASSWORD:-}" ]; then
    echo "==> Submitting for notarization..."
    xcrun notarytool submit "$DMG_PATH" \
        --apple-id "$APPLE_ID" \
        --team-id "$APPLE_TEAM_ID" \
        --password "$APP_PASSWORD" \
        --wait

    echo "==> Stapling notarization ticket..."
    xcrun stapler staple "$DMG_PATH"
    echo "   Notarized and stapled!"
else
    if [ -n "$SIGN_IDENTITY" ]; then
        echo ""
        echo "   Signed but NOT notarized. To notarize, also set:"
        echo "     APPLE_ID, APPLE_TEAM_ID, APP_PASSWORD"
    else
        echo ""
        echo "   To sign + notarize, set: DEVELOPER_ID, APPLE_ID, APPLE_TEAM_ID, APP_PASSWORD"
    fi
fi

echo ""
echo "==> Done!"
echo "   App: $APP_BUNDLE"
echo "   DMG: $DMG_PATH"
echo ""
echo "To run:  open $APP_BUNDLE"
echo "To install: cp -R $APP_BUNDLE /Applications/"
