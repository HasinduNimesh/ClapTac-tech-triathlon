package objectstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func recordingStore(t *testing.T, createStatus int) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.Method == http.MethodPut && r.URL.Path == "/proof" {
			w.WriteHeader(createStatus)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestPutCreatesTheBucketFirstByDefault(t *testing.T) {
	srv, calls := recordingStore(t, http.StatusOK)
	s := S3{Endpoint: srv.URL, Bucket: "proof", AccessKey: "a", SecretKey: "b", Region: "us-east-1"}
	if err := s.Put(context.Background(), "k", "image/jpeg", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 || (*calls)[0] != "PUT /proof" || (*calls)[1] != "PUT /proof/k" {
		t.Fatalf("calls: %v", *calls)
	}
}

// A hosted store such as Cloudflare R2 answers a bucket-scoped key's create-bucket request AccessDenied.
func TestPutFailsWhenTheStoreRefusesToCreateTheBucket(t *testing.T) {
	srv, _ := recordingStore(t, http.StatusForbidden)
	s := S3{Endpoint: srv.URL, Bucket: "proof", AccessKey: "a", SecretKey: "b", Region: "auto"}
	if err := s.Put(context.Background(), "k", "image/jpeg", []byte("x")); err == nil {
		t.Fatal("expected the refused create-bucket request to fail the upload")
	}
}

func TestPutSkipsBucketCreationWhenAsked(t *testing.T) {
	srv, calls := recordingStore(t, http.StatusForbidden)
	s := S3{Endpoint: srv.URL, Bucket: "proof", AccessKey: "a", SecretKey: "b", Region: "auto", SkipBucketCreate: true}
	if err := s.Put(context.Background(), "k", "image/jpeg", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0] != "PUT /proof/k" {
		t.Fatalf("only the object may be written: %v", *calls)
	}
}
