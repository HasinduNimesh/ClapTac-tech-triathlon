package nginx

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

func conf(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("nginx.conf")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Behind Caddy every request arrives from the Docker gateway. Without the real visitor address, one
// rate-limit allowance is shared by every user of the site.
func TestRateLimitUsesTheRealVisitorAddress(t *testing.T) {
	c := conf(t)
	for _, want := range []string{`real_ip_header X-Forwarded-For;`, `set_real_ip_from 172.16.0.0/12;`, `set_real_ip_from 127.0.0.1;`} {
		if !regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(c) {
			t.Fatalf("nginx.conf must contain %q", want)
		}
	}
}

// Pages and static files must not use up the allowance: only /api/ requests get a key, and a request with an
// empty key is not limited.
func TestRateLimitCountsOnlyApiCalls(t *testing.T) {
	c := conf(t)
	if !regexp.MustCompile(`(?s)map \$uri \$api_limit_key \{\s*default\s+"";\s*~\^/api/\s+\$binary_remote_addr;`).MatchString(c) {
		t.Fatal("the limit key must be empty except for /api/ requests")
	}
	if regexp.MustCompile(`limit_req_zone \$binary_remote_addr`).MatchString(c) {
		t.Fatal("the zone must be keyed on $api_limit_key, not on every request")
	}
	if !regexp.MustCompile(`limit_req_zone \$api_limit_key zone=edge:`).MatchString(c) {
		t.Fatal("the edge zone must be keyed on $api_limit_key")
	}
}

// A single page of the web app makes a dozen or more API calls at once, so the allowance has to be generous.
func TestRateLimitIsGenerousEnoughForAPageLoad(t *testing.T) {
	c := conf(t)
	rate := regexp.MustCompile(`zone=edge:\w+ rate=(\d+)r/s`).FindStringSubmatch(c)
	burst := regexp.MustCompile(`limit_req zone=edge burst=(\d+)`).FindStringSubmatch(c)
	if rate == nil || burst == nil {
		t.Fatal("rate and burst not found in nginx.conf")
	}
	if r, _ := strconv.Atoi(rate[1]); r < 50 {
		t.Fatalf("rate %dr/s is too low for a page of API calls", r)
	}
	if b, _ := strconv.Atoi(burst[1]); b < 100 {
		t.Fatalf("burst %d is too low for a page of API calls", b)
	}
}
