package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type user struct {
	Password string
	Subject  string
}

var users = map[string]user{
	"store-manager":   {Password: "waypoint", Subject: "usr-store-manager"},
	"store-manager-b": {Password: "waypoint", Subject: "usr-store-manager-b"},
	"dispatcher":      {Password: "waypoint", Subject: "usr-dispatcher"},
	"loader":          {Password: "waypoint", Subject: "usr-loader"},
	"loader-kandy":    {Password: "waypoint", Subject: "usr-loader-kandy"},
	"driver":          {Password: "waypoint", Subject: "usr-driver"},
}

type codeRec struct {
	Subject  string
	Verifier string
	ClientID string
	Nonce    string
	Scope    string
	Exp      time.Time
}

// refreshRec is a refresh token. Like a real provider, one is issued only when the sign-in asked for
// the offline_access scope, and each use replaces it (rotation) so a stolen old one stops working.
type refreshRec struct {
	Subject  string
	ClientID string
	Scope    string
	Exp      time.Time
}

var (
	key           *rsa.PrivateKey
	kid           = "waypoint-dev"
	issuer        string
	codes         = map[string]codeRec{}
	refreshTokens = map[string]refreshRec{}
	mu            sync.Mutex
)

// machineTokenTTL is how long a service-to-service token lives. The services cache these for about
// an hour, so ACCESS_TOKEN_TTL_SECONDS deliberately does not shorten them.
const machineTokenTTL = time.Hour

// accessTokenTTL is how long a signed-in person's access token lives. Short values (ACCESS_TOKEN_TTL_SECONDS=60) make
// it easy to watch a client refresh.
func accessTokenTTL() time.Duration {
	if n, err := strconv.Atoi(os.Getenv("ACCESS_TOKEN_TTL_SECONDS")); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return time.Hour
}

func main() {
	var err error
	key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}
	issuer = getenv("OIDC_ISSUER", "http://localhost:8090")
	addr := getenv("HTTP_ADDR", ":8090")
	http.HandleFunc("/.well-known/openid-configuration", discovery)
	http.HandleFunc("/oauth2/jwks", jwks)
	http.HandleFunc("/oauth2/authorize", authorize)
	http.HandleFunc("/oauth2/token", token)
	http.HandleFunc("/oauth2/revoke", revoke)
	http.HandleFunc("/admin/users", adminUsers)
	http.HandleFunc("/admin/users.tsv", adminUsersTSV)
	http.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"live"}`))
	})
	log.Printf("dev-oidc listening %s issuer=%s", addr, issuer)
	log.Fatal(http.ListenAndServe(addr, cors(http.DefaultServeMux)))
}

func discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth2/authorize",
		"token_endpoint":                        issuer + "/oauth2/token",
		"revocation_endpoint":                   issuer + "/oauth2/revoke",
		"jwks_uri":                              issuer + "/oauth2/jwks",
		"response_types_supported":              []string{"code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", "client_credentials"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		// Required by OIDC Discovery; native libraries such as AppAuth reject the document without it.
		"subject_types_supported":               []string{"public"},
		"scopes_supported":                      []string{"openid", "profile", "offline_access"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post"},
	})
}

func jwks(w http.ResponseWriter, _ *http.Request) {
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1})
	writeJSON(w, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig", "n": n, "e": e,
	}}})
}

func authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body><h1>Waypoint local identity</h1>
<form method="post">
<input type="hidden" name="redirect_uri" value="%s"/>
<input type="hidden" name="state" value="%s"/>
<input type="hidden" name="code_challenge" value="%s"/>
<input type="hidden" name="client_id" value="%s"/>
<input type="hidden" name="nonce" value="%s"/>
<input type="hidden" name="scope" value="%s"/>
<p>username <input name="username"/></p>
<p>password <input type="password" name="password"/></p>
<button type="submit">Sign in</button>
</form>
<p>store-manager / dispatcher / loader / loader-kandy / driver — password: waypoint</p>
</body></html>`, q.Get("redirect_uri"), q.Get("state"), q.Get("code_challenge"), q.Get("client_id"), q.Get("nonce"), q.Get("scope"))
		return
	}
	_ = r.ParseForm()
	u, ok := users[r.FormValue("username")]
	if !ok || u.Password != r.FormValue("password") {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	code := random(16)
	mu.Lock()
	codes[code] = codeRec{
		Subject:  u.Subject,
		Verifier: r.FormValue("code_challenge"),
		ClientID: r.FormValue("client_id"),
		Nonce:    r.FormValue("nonce"),
		Scope:    r.FormValue("scope"),
		Exp:      time.Now().Add(5 * time.Minute),
	}
	mu.Unlock()
	redir := r.FormValue("redirect_uri")
	if redir == "" {
		redir = q.Get("redirect_uri")
	}
	http.Redirect(w, r, fmt.Sprintf("%s?code=%s&state=%s", redir, code, r.FormValue("state")), http.StatusFound)
}

func token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	log.Printf("token grant_type=%s client_id=%s", r.FormValue("grant_type"), r.FormValue("client_id"))
	switch r.FormValue("grant_type") {
	case "authorization_code":
		mu.Lock()
		rec, ok := codes[r.FormValue("code")]
		delete(codes, r.FormValue("code"))
		mu.Unlock()
		if !ok || time.Now().After(rec.Exp) {
			http.Error(w, "invalid_grant", http.StatusBadRequest)
			return
		}
		if rec.Verifier != "" && s256(r.FormValue("code_verifier")) != rec.Verifier {
			http.Error(w, "invalid_grant", http.StatusBadRequest)
			return
		}
		tok, err := sign(rec.Subject, "openid profile", getenv("OIDC_AUDIENCE", "waypoint-api"), nil)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// The ID token is for the client (aud = client_id) and echoes the nonce, as native
		// OIDC libraries such as AppAuth require. The access token stays scoped to the API.
		idClaims := map[string]any{"nonce": rec.Nonce, "azp": rec.ClientID}
		idClient := rec.ClientID
		if idClient == "" {
			idClient = getenv("OIDC_AUDIENCE", "waypoint-api")
		}
		idTok, err := sign(rec.Subject, "openid profile", idClient, idClaims)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		resp := map[string]any{"access_token": tok, "token_type": "Bearer", "expires_in": int(accessTokenTTL().Seconds()), "id_token": idTok}
		if hasScope(rec.Scope, "offline_access") {
			resp["refresh_token"] = newRefreshToken(rec.Subject, rec.ClientID, rec.Scope)
			resp["scope"] = rec.Scope
		}
		writeJSON(w, resp)
	case "refresh_token":
		mu.Lock()
		rec, ok := refreshTokens[r.FormValue("refresh_token")]
		// Rotation: a refresh token works once.
		delete(refreshTokens, r.FormValue("refresh_token"))
		mu.Unlock()
		if !ok || time.Now().After(rec.Exp) {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token is not valid")
			return
		}
		if c := r.FormValue("client_id"); c != "" && rec.ClientID != "" && c != rec.ClientID {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token was issued to another client")
			return
		}
		tok, err := sign(rec.Subject, "openid profile", getenv("OIDC_AUDIENCE", "waypoint-api"), nil)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{
			"access_token":  tok,
			"token_type":    "Bearer",
			"expires_in":    int(accessTokenTTL().Seconds()),
			"refresh_token": newRefreshToken(rec.Subject, rec.ClientID, rec.Scope),
			"scope":         rec.Scope,
		})
	case "client_credentials":
		if r.FormValue("client_id") == "" {
			http.Error(w, "invalid_client", http.StatusUnauthorized)
			return
		}
		tok, err := signFor("svc-"+r.FormValue("client_id"), r.FormValue("scope"), getenv("OIDC_AUDIENCE", "waypoint-api"), nil, machineTokenTTL)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"access_token": tok, "token_type": "Bearer", "expires_in": 3600})
	default:
		http.Error(w, "unsupported_grant_type", http.StatusBadRequest)
	}
}

func sign(sub, scope, aud string, extra map[string]any) (string, error) {
	return signFor(sub, scope, aud, extra, accessTokenTTL())
}

// signFor signs a token that lives for ttl.
func signFor(sub, scope, aud string, extra map[string]any, ttl time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"sub": sub, "iss": issuer, "aud": aud,
		"exp": time.Now().Add(ttl).Unix(), "iat": time.Now().Unix(),
		"scope": scope,
	}
	for k, v := range extra {
		if v != "" {
			claims[k] = v
		}
	}
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	t.Header["kid"] = kid
	return t.SignedString(key)
}

// revoke implements RFC 7009 for refresh tokens: the token is removed and the answer is 200 whether or
// not it was known (so a client cannot probe for valid tokens). A request without a token is an error.
func revoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = r.ParseForm()
	token := r.FormValue("token")
	if token == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "token is required")
		return
	}
	log.Printf("revoke token_type_hint=%s client_id=%s", r.FormValue("token_type_hint"), r.FormValue("client_id"))
	mu.Lock()
	delete(refreshTokens, token)
	mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func hasScope(scope, want string) bool {
	for _, s := range strings.Fields(scope) {
		if s == want {
			return true
		}
	}
	return false
}

func newRefreshToken(subject, clientID, scope string) string {
	token := random(32)
	mu.Lock()
	refreshTokens[token] = refreshRec{Subject: subject, ClientID: clientID, Scope: scope, Exp: time.Now().Add(30 * 24 * time.Hour)}
	mu.Unlock()
	return token
}

// oauthError answers as RFC 6749 section 5.2 describes, which native libraries parse.
func oauthError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": description})
}

func s256(v string) string {
	sum := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func random(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

type provisionedUser struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Subject  string `json:"subject"`
	Role     string `json:"role"`
	OutletID string `json:"outletId,omitempty"`
	Depot    string `json:"depot,omitempty"`
	Vehicle  string `json:"vehicleId,omitempty"`
}

func provisioned() []provisionedUser {
	return []provisionedUser{
		{UserID: "USR001", Username: "store-manager", Subject: users["store-manager"].Subject, Role: "STORE_MANAGER", OutletID: "OUT034"},
		{UserID: "USR002", Username: "dispatcher", Subject: users["dispatcher"].Subject, Role: "DISPATCHER", Depot: "DEPOT_NORTH"},
		{UserID: "USR003", Username: "store-manager-b", Subject: users["store-manager-b"].Subject, Role: "STORE_MANAGER", OutletID: "OUT021"},
		{UserID: "USR004", Username: "loader", Subject: users["loader"].Subject, Role: "LOADER", Depot: "DEPOT_NORTH"},
		{UserID: "USR005", Username: "loader-kandy", Subject: users["loader-kandy"].Subject, Role: "LOADER", Depot: "DEPOT_SOUTH"},
		{UserID: "USR006", Username: "driver", Subject: users["driver"].Subject, Role: "DRIVER", Vehicle: "VEH001"},
	}
}

func adminUsers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"users": provisioned()})
}

func adminUsersTSV(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/tab-separated-values")
	fmt.Fprintln(w, "user_id\tsubject\trole\toutlet_id\tdepot\tvehicle_id")
	for _, u := range provisioned() {
		outlet, depot, vehicle := u.OutletID, u.Depot, u.Vehicle
		if outlet == "" {
			outlet = "-"
		}
		if depot == "" {
			depot = "-"
		}
		if vehicle == "" {
			vehicle = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", u.UserID, u.Subject, u.Role, outlet, depot, vehicle)
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
