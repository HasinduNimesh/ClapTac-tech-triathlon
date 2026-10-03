package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func setup(t *testing.T) {
	t.Helper()
	var err error
	key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer = "http://issuer.test"
	codes = map[string]codeRec{}
}

func claimsOf(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

// Native OIDC libraries such as AppAuth refuse a discovery document that omits these fields.
func TestDiscoveryHasTheFieldsNativeClientsRequire(t *testing.T) {
	setup(t)
	rec := httptest.NewRecorder()
	discovery(rec, httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil))
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"issuer", "authorization_endpoint", "token_endpoint", "jwks_uri", "response_types_supported", "subject_types_supported", "id_token_signing_alg_values_supported"} {
		if _, ok := doc[field]; !ok {
			t.Errorf("discovery document is missing %q", field)
		}
	}
	if doc["issuer"] != "http://issuer.test" {
		t.Errorf("issuer = %v", doc["issuer"])
	}
}

func signIn(t *testing.T, clientID, nonce string) (accessToken, idToken string) {
	t.Helper()
	verifier := "a-long-random-code-verifier-for-the-test-0123456789"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	form := url.Values{
		"redirect_uri":   {"dev.claptac.waypointdriver:/oauth2redirect"},
		"state":          {"st"},
		"code_challenge": {challenge},
		"client_id":      {clientID},
		"nonce":          {nonce},
		"username":       {"driver"},
		"password":       {"waypoint"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	authorize(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("authorize status = %d: %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "dev.claptac.waypointdriver:/oauth2redirect?") {
		t.Fatalf("redirect = %q", location)
	}
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	code := parsed.Query().Get("code")

	tokenForm := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "client_id": {clientID}}
	tokenReq := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(tokenForm.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenRec := httptest.NewRecorder()
	token(tokenRec, tokenReq)
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("token status = %d: %s", tokenRec.Code, tokenRec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(tokenRec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body["access_token"].(string), body["id_token"].(string)
}

func TestIDTokenIsForTheClientAndEchoesTheNonceWhileTheAccessTokenStaysForTheAPI(t *testing.T) {
	setup(t)
	access, id := signIn(t, "waypoint-driver", "n-123")

	idClaims := claimsOf(t, id)
	if idClaims["aud"] != "waypoint-driver" {
		t.Errorf("id_token aud = %v, want the client id", idClaims["aud"])
	}
	if idClaims["nonce"] != "n-123" {
		t.Errorf("id_token nonce = %v, want n-123", idClaims["nonce"])
	}
	if idClaims["sub"] != "usr-driver" {
		t.Errorf("id_token sub = %v", idClaims["sub"])
	}

	accessClaims := claimsOf(t, access)
	if accessClaims["aud"] != "waypoint-api" {
		t.Errorf("access token aud = %v, want waypoint-api", accessClaims["aud"])
	}
	if _, has := accessClaims["nonce"]; has {
		t.Error("the access token must not carry the nonce")
	}
}

func TestNoNonceMeansNoNonceClaim(t *testing.T) {
	setup(t)
	_, id := signIn(t, "waypoint-web", "")
	claims := claimsOf(t, id)
	if _, has := claims["nonce"]; has {
		t.Errorf("unexpected nonce claim %v", claims["nonce"])
	}
	if claims["aud"] != "waypoint-web" {
		t.Errorf("aud = %v", claims["aud"])
	}
}
