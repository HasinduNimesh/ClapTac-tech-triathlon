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
	refreshTokens = map[string]refreshRec{}
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
	body := signInWith(t, clientID, nonce, "openid profile")
	return body["access_token"].(string), body["id_token"].(string)
}

// signInWith runs the authorization-code flow with the given scopes and returns the whole token response.
func signInWith(t *testing.T, clientID, nonce, scope string) map[string]any {
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
		"scope":          {scope},
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
	return body
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

func postToken(t *testing.T, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	token(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not JSON: %q", rec.Body.String())
	}
	return body
}

func TestDiscoveryAdvertisesRefreshTokens(t *testing.T) {
	setup(t)
	rec := httptest.NewRecorder()
	discovery(rec, httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil))
	doc := decode(t, rec)
	grants, _ := doc["grant_types_supported"].([]any)
	scopes, _ := doc["scopes_supported"].([]any)
	if !containsAny(grants, "refresh_token") {
		t.Errorf("grant_types_supported = %v", grants)
	}
	if !containsAny(scopes, "offline_access") {
		t.Errorf("scopes_supported = %v", scopes)
	}
}

func containsAny(list []any, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestARefreshTokenIsIssuedOnlyWhenOfflineAccessWasRequested(t *testing.T) {
	setup(t)
	without := signInWith(t, "waypoint-driver", "n", "openid profile")
	if _, has := without["refresh_token"]; has {
		t.Error("a refresh token was issued without offline_access")
	}
	with := signInWith(t, "waypoint-driver", "n", "openid profile offline_access")
	if rt, _ := with["refresh_token"].(string); rt == "" {
		t.Error("no refresh token for offline_access")
	}
}

func TestRefreshGrantIssuesANewAccessTokenAndRotatesTheRefreshToken(t *testing.T) {
	setup(t)
	first := signInWith(t, "waypoint-driver", "n", "openid profile offline_access")
	old := first["refresh_token"].(string)

	rec := postToken(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {old}, "client_id": {"waypoint-driver"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	access := body["access_token"].(string)
	claims := claimsOf(t, access)
	if claims["sub"] != "usr-driver" || claims["aud"] != "waypoint-api" {
		t.Errorf("claims = %v", claims)
	}
	next := body["refresh_token"].(string)
	if next == "" || next == old {
		t.Errorf("refresh token was not rotated: %q", next)
	}
	if body["expires_in"].(float64) <= 0 {
		t.Errorf("expires_in = %v", body["expires_in"])
	}

	// The new one works; the old one does not.
	if rec := postToken(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {next}}); rec.Code != http.StatusOK {
		t.Errorf("the rotated token should work once, got %d", rec.Code)
	}
}

func TestAUsedRefreshTokenIsRejectedAsInvalidGrantInJSON(t *testing.T) {
	setup(t)
	old := signInWith(t, "waypoint-driver", "n", "openid offline_access")["refresh_token"].(string)
	postToken(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {old}})

	rec := postToken(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {old}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := decode(t, rec)["error"]; got != "invalid_grant" {
		t.Errorf("error = %v, want invalid_grant", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content type = %q", ct)
	}
}

func TestUnknownAndOtherClientsRefreshTokensAreRejected(t *testing.T) {
	setup(t)
	if rec := postToken(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"nope"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown token status = %d", rec.Code)
	}
	rt := signInWith(t, "waypoint-driver", "n", "openid offline_access")["refresh_token"].(string)
	rec := postToken(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {rt}, "client_id": {"someone-else"}})
	if rec.Code != http.StatusBadRequest || decode(t, rec)["error"] != "invalid_grant" {
		t.Errorf("other client: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAccessTokenLifetimeCanBeShortenedForTesting(t *testing.T) {
	setup(t)
	t.Setenv("ACCESS_TOKEN_TTL_SECONDS", "60")
	body := signInWith(t, "waypoint-driver", "n", "openid offline_access")
	if body["expires_in"].(float64) != 60 {
		t.Errorf("expires_in = %v", body["expires_in"])
	}
	claims := claimsOf(t, body["access_token"].(string))
	if remaining := claims["exp"].(float64) - claims["iat"].(float64); remaining != 60 {
		t.Errorf("exp-iat = %v", remaining)
	}
	t.Setenv("ACCESS_TOKEN_TTL_SECONDS", "")
	if got := accessTokenTTL().Seconds(); got != 3600 {
		t.Errorf("default ttl = %v", got)
	}
}
