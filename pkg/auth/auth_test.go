package auth

import "testing"

func TestValidateDisabledGuard(t *testing.T) {
	t.Parallel()

	if err := ValidateDisabledGuard("local", true); err != nil {
		t.Fatalf("local + AUTH_DISABLED should be allowed: %v", err)
	}
	if err := ValidateDisabledGuard("local", false); err != nil {
		t.Fatalf("local with auth enabled should be allowed: %v", err)
	}
	if err := ValidateDisabledGuard("staging", true); err == nil {
		t.Fatal("staging + AUTH_DISABLED must fail startup")
	}
	if err := ValidateDisabledGuard("production", true); err == nil {
		t.Fatal("production + AUTH_DISABLED must fail startup")
	}
	if err := ValidateDisabledGuard("", true); err == nil {
		t.Fatal("empty environment + AUTH_DISABLED must fail startup")
	}
	if err := ValidateDisabledGuard("production", false); err != nil {
		t.Fatalf("production with auth enabled should be allowed: %v", err)
	}
}
