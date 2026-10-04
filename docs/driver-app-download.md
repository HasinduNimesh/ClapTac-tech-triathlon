# Driver app download

The home page (`/`) offers the Android driver app. It reads `/downloads/waypoint-driver.json`; when that file
is valid it shows the version, size, build date and SHA-256 with a download button for
`/downloads/waypoint-driver.apk`. When the file is missing the page says the app has not been published yet.

nginx serves `/srv/downloads/`, mounted read-only from `${DOWNLOADS_DIR:-./downloads}` next to `docker-compose.yml`.
The APK itself is never committed (`/downloads/*` is ignored).

## Publish a build

1. On a machine with Flutter and the Android SDK: `scripts/build-driver-apk.sh`.
   It builds a release APK against the production issuer and API and writes the APK and the JSON into `downloads/`.
2. Copy `downloads/` to the server: `scp downloads/waypoint-driver.* azureuser@<vm>:<repo>/downloads/`.
3. No restart is needed. The JSON is served with `Cache-Control: no-cache`.

## Signing

Without `apps/driver-mobile/android/key.properties` the release APK is signed with the debug key. Phones that
installed it can only update from an APK signed with the same key, so create a release keystore before handing
the app to drivers, and keep it out of git.
