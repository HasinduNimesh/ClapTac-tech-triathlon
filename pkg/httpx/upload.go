package httpx

import (
	"errors"
	"io"
	"net/http"

	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
)

// multipartOverhead is the room a multipart envelope needs beyond the file itself.
const multipartOverhead = 256 << 10

// ReadImageUpload reads the single "file" part of a multipart request. It caps the
// request at maxBytes plus the multipart envelope, reads at most maxBytes+1 of the
// file (so the caller can tell "too big" from "exactly the limit"), and returns the
// part's Content-Type, falling back to the "mimeType" form value when the part sends
// none. On any problem it writes the problem response and returns ok=false. Form
// values such as "type" or "capturedAt" stay readable with r.FormValue afterwards.
//
// Callers still validate the content type and the file signature (see
// validation.HasImageMagic) and the size limit: this only does the HTTP handling
// that photo and proof uploads have in common.
func ReadImageUpload(w http.ResponseWriter, r *http.Request, maxBytes int64) (body []byte, contentType string, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+multipartOverhead)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			apierrors.RequestEntityTooLarge(w, "upload exceeds the request size limit")
			return nil, "", false
		}
		apierrors.BadRequest(w, "multipart upload required")
		return nil, "", false
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		apierrors.BadRequest(w, "file is required")
		return nil, "", false
	}
	defer file.Close()
	body, err = io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		apierrors.BadRequest(w, "unable to read file")
		return nil, "", false
	}
	contentType = hdr.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = r.FormValue("mimeType")
	}
	return body, contentType, true
}
