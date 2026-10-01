package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var allowedAlgs = map[string]bool{
	"RS256": true,
}

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type JWKSCache struct {
	URL    string
	Client *http.Client
	mu     sync.RWMutex
	keys   map[string]*rsa.PublicKey
	exp    time.Time
}

func NewJWKSCache(url string) *JWKSCache {
	return &JWKSCache{
		URL:    url,
		Client: &http.Client{Timeout: 10 * time.Second},
		keys:   map[string]*rsa.PublicKey{},
	}
}

func (c *JWKSCache) Keyfunc(token *jwt.Token) (any, error) {
	alg, _ := token.Header["alg"].(string)
	if !allowedAlgs[alg] {
		return nil, fmt.Errorf("signing algorithm %q is not allowed", alg)
	}
	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("token missing kid")
	}
	key, err := c.lookup(kid)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (c *JWKSCache) lookup(kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	if time.Now().Before(c.exp) {
		if k, ok := c.keys[kid]; ok {
			c.mu.RUnlock()
			return k, nil
		}
	}
	c.mu.RUnlock()
	if err := c.refresh(); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	k, ok := c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("unknown kid %q", kid)
	}
	return k, nil
}

func (c *JWKSCache) refresh() error {
	resp, err := c.Client.Get(c.URL)
	if err != nil {
		return fmt.Errorf("jwks fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks fetch: status %d", resp.StatusCode)
	}
	const maxJWKSBytes = 1 << 20
	if resp.ContentLength > maxJWKSBytes {
		return errors.New("jwks document too large")
	}
	var doc jwksDocument
	limited := &io.LimitedReader{R: resp.Body, N: maxJWKSBytes + 1}
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(&doc); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("invalid or oversized jwks document")
	}
	if limited.N == 0 {
		return errors.New("jwks document too large")
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" || (k.Alg != "" && k.Alg != "RS256") || (k.Use != "" && k.Use != "sig") {
			continue
		}
		pub, err := rsaPublic(k)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	c.mu.Lock()
	c.keys = keys
	c.exp = time.Now().Add(5 * time.Minute)
	c.mu.Unlock()
	return nil
}

func rsaPublic(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	n := new(big.Int).SetBytes(nBytes)
	if n.BitLen() < 2048 || e < 3 || e%2 == 0 {
		return nil, errors.New("invalid RSA public key parameters")
	}
	return &rsa.PublicKey{N: n, E: e}, nil
}

func NewJWTAuthenticator(issuer, audience, jwksURL string) JWTAuthenticator {
	cache := NewJWKSCache(jwksURL)
	return JWTAuthenticator{
		Issuer:   issuer,
		Audience: audience,
		JWKSURL:  jwksURL,
		KeyFunc:  cache.Keyfunc,
	}
}
