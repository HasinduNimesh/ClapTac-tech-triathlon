package httpx

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
)

func upload(t *testing.T, partType string, fields map[string]string, file []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="p"`)
	if partType != "" {
		h.Set("Content-Type", partType)
	}
	part, _ := mw.CreatePart(h)
	_, _ = part.Write(file)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestReadImageUploadReturnsTheFileAndItsType(t *testing.T) {
	rec := httptest.NewRecorder()
	body, ct, ok := ReadImageUpload(rec, upload(t, "image/jpeg", nil, []byte("abc")), 10)
	if !ok || string(body) != "abc" || ct != "image/jpeg" {
		t.Fatalf("got %q %q %v (%d)", body, ct, ok, rec.Code)
	}
}

func TestReadImageUploadFallsBackToTheMimeTypeField(t *testing.T) {
	for _, partType := range []string{"", "application/octet-stream"} {
		rec := httptest.NewRecorder()
		_, ct, ok := ReadImageUpload(rec, upload(t, partType, map[string]string{"mimeType": "image/png"}, []byte("abc")), 10)
		if !ok || ct != "image/png" {
			t.Fatalf("part type %q: got %q %v", partType, ct, ok)
		}
	}
}

func TestReadImageUploadRefusesWhatIsNotAMultipartFile(t *testing.T) {
	rec := httptest.NewRecorder()
	if _, _, ok := ReadImageUpload(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("x"))), 10); ok || rec.Code != http.StatusBadRequest {
		t.Fatalf("plain body: ok=%v code=%d", ok, rec.Code)
	}
}

func TestReadImageUploadReadsOneByteBeyondTheLimitSoTheCallerCanRefuseIt(t *testing.T) {
	rec := httptest.NewRecorder()
	body, _, ok := ReadImageUpload(rec, upload(t, "image/png", nil, bytes.Repeat([]byte{1}, 50)), 10)
	if !ok || len(body) != 11 {
		t.Fatalf("expected 11 bytes (limit+1), got %d ok=%v", len(body), ok)
	}
}

func TestReadImageUploadRefusesAnOversizedRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	if _, _, ok := ReadImageUpload(rec, upload(t, "image/png", nil, bytes.Repeat([]byte{1}, 2<<20)), 10); ok || rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized: ok=%v code=%d", ok, rec.Code)
	}
}
