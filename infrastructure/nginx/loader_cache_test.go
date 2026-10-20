package nginx

import (
	"os"
	"regexp"
	"testing"
)

// The loader app's files (main.dart.js and the rest) keep their names between releases. Without a cache header a
// browser guesses how long to keep them, and a loader tablet can run the previous release for days after a deploy.
func TestLoaderAppFilesAreRevalidatedOnEveryLoad(t *testing.T) {
	b, err := os.ReadFile("../../apps/loader-web/nginx.default.conf")
	if err != nil {
		t.Fatal(err)
	}
	location := regexp.MustCompile(`(?s)location /loader-app/ \{(.*?)\n  \}`).FindStringSubmatch(string(b))
	if location == nil {
		t.Fatal("the /loader-app/ location is missing")
	}
	if !regexp.MustCompile(`add_header Cache-Control "no-cache";`).MatchString(location[1]) {
		t.Fatal("everything under /loader-app/ must be sent with Cache-Control: no-cache so a new release is picked up")
	}
}
