package validation

import "testing"

func TestHasImageMagic(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), 1, 2, 3)
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 1}
	if !HasImageMagic("image/png", png) || !HasImageMagic("image/jpeg", jpeg) {
		t.Fatal("real signatures must pass")
	}
	if HasImageMagic("image/png", jpeg) || HasImageMagic("image/jpeg", png) {
		t.Fatal("a signature must match the stated type")
	}
	if HasImageMagic("image/png", []byte("\x89PNG")) || HasImageMagic("image/webp", png) || HasImageMagic("image/jpeg", nil) {
		t.Fatal("short, unknown and empty input must fail")
	}
}
