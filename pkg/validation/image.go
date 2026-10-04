package validation

import "bytes"

var (
	pngSignature  = []byte("\x89PNG\r\n\x1a\n")
	jpegSignature = []byte{0xff, 0xd8, 0xff}
)

// HasImageMagic reports whether b starts with the file signature of the image
// type named by mime ("image/png" or "image/jpeg"). It is the shared check for
// uploaded photos, so every service accepts exactly the same formats.
func HasImageMagic(mime string, b []byte) bool {
	switch mime {
	case "image/png":
		return bytes.HasPrefix(b, pngSignature)
	case "image/jpeg":
		return bytes.HasPrefix(b, jpegSignature)
	}
	return false
}
