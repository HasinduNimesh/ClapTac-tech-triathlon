package nginx

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// policyOf returns the Permissions-Policy value set by the n-th add_header for it in nginx.conf (0 = the
// server-level one that the web app is served with, 1 = the loader app's location).
func policyOf(t *testing.T, n int) string {
	t.Helper()
	conf, err := os.ReadFile("nginx.conf")
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`add_header Permissions-Policy "([^"]*)"`).FindAllStringSubmatch(string(conf), -1)
	if len(found) <= n {
		t.Fatalf("expected at least %d Permissions-Policy headers in nginx.conf, found %d", n+1, len(found))
	}
	return found[n][1]
}

// The order helper's speech input needs the microphone, so the web app's headers must allow it for
// the site itself (a policy of microphone=() makes the button fail with a permission error).
func TestWebAppMayUseTheMicrophone(t *testing.T) {
	if policy := policyOf(t, 0); !strings.Contains(policy, "microphone=(self)") {
		t.Fatalf("the web app's Permissions-Policy must allow the microphone for the site, got %q", policy)
	}
}

// The loader app has no use for the microphone and keeps it off.
func TestLoaderAppKeepsTheMicrophoneOff(t *testing.T) {
	if policy := policyOf(t, 1); !strings.Contains(policy, "microphone=()") {
		t.Fatalf("the loader app's Permissions-Policy must keep the microphone off, got %q", policy)
	}
}
