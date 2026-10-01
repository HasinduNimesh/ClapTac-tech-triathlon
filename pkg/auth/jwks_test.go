package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWKSKidAndAlgorithm(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{"kty": "RSA", "kid": "k1", "alg": "RS256", "n": n, "e": e}},
		})
	}))
	defer srv.Close()

	authn := NewJWTAuthenticator("https://issuer.test", "waypoint-api", srv.URL)
	makeTok := func(kid, alg string, method jwt.SigningMethod, exp time.Time) string {
		tok := jwt.NewWithClaims(method, jwt.MapClaims{
			"sub": "usr-store-manager", "iss": "https://issuer.test", "aud": "waypoint-api",
			"exp": exp.Unix(),
		})
		tok.Header["kid"] = kid
		tok.Header["alg"] = alg
		s, err := tok.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	ok := makeTok("k1", "RS256", jwt.SigningMethodRS256, time.Now().Add(time.Hour))
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+ok)
	if _, err := authn.Authenticate(req); err != nil {
		t.Fatalf("valid token: %v", err)
	}

	badKid := makeTok("other", "RS256", jwt.SigningMethodRS256, time.Now().Add(time.Hour))
	req.Header.Set("Authorization", "Bearer "+badKid)
	if _, err := authn.Authenticate(req); err == nil {
		t.Fatal("expected unknown kid")
	}

	expired := makeTok("k1", "RS256", jwt.SigningMethodRS256, time.Now().Add(-time.Hour))
	req.Header.Set("Authorization", "Bearer "+expired)
	if _, err := authn.Authenticate(req); err == nil {
		t.Fatal("expected expiry failure")
	}

	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "usr-store-manager", "iss": "https://issuer.test", "aud": "waypoint-api",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	hs.Header["kid"] = "k1"
	hsTok, err := hs.SignedString([]byte("not-rsa"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+hsTok)
	if _, err := authn.Authenticate(req); err == nil {
		t.Fatal("expected unexpected alg rejection")
	}

	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "usr-store-manager", "iss": "https://issuer.test", "aud": "waypoint-api",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	none.Header["kid"] = "k1"
	noneTok, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+noneTok)
	if _, err := authn.Authenticate(req); err == nil {
		t.Fatal("expected none alg rejection")
	}
}
