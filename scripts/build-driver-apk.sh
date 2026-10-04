#!/usr/bin/env bash
# Builds the Android driver app and writes downloads/waypoint-driver.apk plus
# downloads/waypoint-driver.json, which the home page reads to offer the download.
# Run on a machine with Flutter and the Android SDK. Copy ./downloads to the server afterwards.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
app="$root/apps/driver-mobile"
out="${DOWNLOADS_DIR:-$root/downloads}"

OIDC_ISSUER="${OIDC_ISSUER:-https://id.waypoint.claptac.dev}"
API_BASE_URL="${API_BASE_URL:-https://waypoint.claptac.dev}"
OIDC_RESOURCE="${OIDC_RESOURCE:-https://waypoint.claptac.dev/api/v1}"
OIDC_SCOPES="${OIDC_SCOPES:-openid profile offline_access}"

if [ ! -f "$app/android/key.properties" ]; then
  echo "warning: android/key.properties is missing, so this APK is signed with the debug key." >&2
  echo "         Phones will not be able to update it from an APK signed with a different key." >&2
fi

cd "$app"
flutter build apk --release \
  --dart-define=OIDC_ISSUER="$OIDC_ISSUER" \
  --dart-define=API_BASE_URL="$API_BASE_URL" \
  --dart-define=OIDC_RESOURCE="$OIDC_RESOURCE" \
  --dart-define=OIDC_SCOPES="$OIDC_SCOPES"

apk="$app/build/app/outputs/flutter-apk/app-release.apk"
mkdir -p "$out"
cp "$apk" "$out/waypoint-driver.apk"

version="$(sed -n 's/^version:[[:space:]]*//p' pubspec.yaml | head -1)"
size="$(wc -c < "$out/waypoint-driver.apk" | tr -d ' ')"
sha="$(sha256sum "$out/waypoint-driver.apk" | cut -d' ' -f1)"
commit="$(git -C "$root" rev-parse --short HEAD 2>/dev/null || echo unknown)"
built="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

cat > "$out/waypoint-driver.json" <<JSON
{"version":"$version","size":$size,"sha256":"$sha","builtAt":"$built","commit":"$commit"}
JSON

echo "wrote $out/waypoint-driver.apk ($size bytes, sha256 $sha)"
