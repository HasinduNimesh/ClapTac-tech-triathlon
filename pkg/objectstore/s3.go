package objectstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Store interface {
	Put(ctx context.Context, key, contentType string, body []byte) error
	Delete(ctx context.Context, key string) error
}

type Memory struct {
	mu      sync.Mutex
	Objects map[string][]byte
}

func (m *Memory) Put(_ context.Context, key, _ string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Objects == nil {
		m.Objects = map[string][]byte{}
	}
	cp := make([]byte, len(body))
	copy(cp, body)
	m.Objects[key] = cp
	return nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Objects, key)
	return nil
}

type S3 struct {
	Endpoint         string
	Bucket           string
	AccessKey        string
	SecretKey        string
	Region           string
	AutoCreateBucket bool
	HTTP             *http.Client
}

func (s S3) http() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return http.DefaultClient
}

func (s S3) Put(ctx context.Context, key, contentType string, body []byte) error {
	if s.AutoCreateBucket {
		if err := s.ensureBucket(ctx); err != nil {
			return err
		}
	}
	return s.do(ctx, http.MethodPut, key, contentType, body)
}

func (s S3) Delete(ctx context.Context, key string) error {
	return s.do(ctx, http.MethodDelete, key, "", nil)
}

func (s S3) ensureBucket(ctx context.Context) error {
	if s.Endpoint == "" || s.Bucket == "" {
		return fmt.Errorf("object store not configured")
	}
	base, err := url.Parse(s.Endpoint)
	if err != nil {
		return err
	}
	u := *base
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + pathEscape(s.Bucket)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), nil)
	if err != nil {
		return err
	}
	sign(req, []byte{}, s.AccessKey, s.SecretKey, s.region(), u.Host, "/"+pathEscape(s.Bucket))
	resp, err := s.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusMethodNotAllowed {
		return nil
	}
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusBadRequest && strings.Contains(string(b), "BucketAlready") {
		return nil
	}
	return fmt.Errorf("s3 create bucket: %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
}

func (s S3) do(ctx context.Context, method, key, contentType string, body []byte) error {
	if s.Endpoint == "" || s.Bucket == "" {
		return fmt.Errorf("object store not configured")
	}
	base, err := url.Parse(s.Endpoint)
	if err != nil {
		return err
	}
	escaped := pathEscape(s.Bucket) + "/" + pathEscape(key)
	u := *base
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + escaped
	if body == nil {
		body = []byte{}
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	sign(req, body, s.AccessKey, s.SecretKey, s.region(), u.Host, "/"+escaped)
	resp, err := s.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3 %s %s: %d %s", method, key, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (s S3) region() string {
	if s.Region != "" {
		return s.Region
	}
	return "us-east-1"
}

func pathEscape(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func sign(req *http.Request, body []byte, access, secret, region, host, canonicalURI string) {
	if access == "" {
		access = "s3mock"
	}
	if secret == "" {
		secret = "s3mock"
	}
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payload := sha256.Sum256(body)
	payloadHex := hex.EncodeToString(payload[:])
	req.Header.Set("Host", host)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHex)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:" + host + "\n" + "x-amz-content-sha256:" + payloadHex + "\n" + "x-amz-date:" + amzDate + "\n"
	canonical := strings.Join([]string{req.Method, canonicalURI, "", canonicalHeaders, signedHeaders, payloadHex}, "\n")
	scope := dateStamp + "/" + region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", amzDate, scope, hex.EncodeToString(sum[:])}, "\n")
	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", access, scope, signedHeaders, sig))
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(data))
	return m.Sum(nil)
}
