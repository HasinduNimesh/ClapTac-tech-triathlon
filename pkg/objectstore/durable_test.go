package objectstore

import "testing"

func TestInMemoryProofStorageIsOnlyForLocalDevelopment(t *testing.T) {
	if err := RequireDurable(true, ""); err != nil {
		t.Fatalf("local development may use the in-memory store: %v", err)
	}
	if err := RequireDurable(false, ""); err == nil {
		t.Fatal("a deployed service with no MINIO_ENDPOINT must refuse to start, not keep proof in memory")
	}
	if err := RequireDurable(false, "https://store.example"); err != nil {
		t.Fatalf("a configured endpoint is fine anywhere: %v", err)
	}
}
