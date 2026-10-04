package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// How a load line was confirmed. A loader scans the label; if it cannot be
// scanned the order ID may be typed instead, but only with a reason.
const (
	EntryScan   = "SCAN"
	EntryManual = "MANUAL"
	// EntryUnknown is reported for lines loaded without a method (older clients).
	EntryUnknown = "UNKNOWN"

	ReasonDamagedLabel = "DAMAGED_LABEL"
	ReasonUnreadable   = "UNREADABLE"
	ReasonNoCamera     = "NO_CAMERA"
	ReasonOther        = "OTHER"

	// MaxManualNoteLength caps the optional note on a manual entry.
	MaxManualNoteLength = 200
)

// LoadEntry is how one load line was confirmed. The zero value is "unknown":
// the client sent nothing.
type LoadEntry struct {
	Method     string
	ReasonCode string
	Note       string
}

// Manual reports whether the order ID was typed instead of scanned.
func (e LoadEntry) Manual() bool { return e.Method == EntryManual }

// ValidManualReason reports whether code is one of the allowed reasons.
func ValidManualReason(code string) bool {
	switch code {
	case ReasonDamagedLabel, ReasonUnreadable, ReasonNoCamera, ReasonOther:
		return true
	}
	return false
}

// NormalizeEntry checks and cleans the entry a client sent with "mark loaded".
// Nothing at all is accepted (legacy clients). MANUAL needs a reason code from
// the allowed set; the reason and note are refused on any other method.
// Errors start with "invalid" so the handler answers 400.
func NormalizeEntry(method, reasonCode, note string) (LoadEntry, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	reasonCode = strings.ToUpper(strings.TrimSpace(reasonCode))
	note = strings.TrimSpace(note)
	switch method {
	case "":
		if reasonCode != "" || note != "" {
			return LoadEntry{}, fmt.Errorf("invalid: entryMethod is required when reasonCode or note is sent")
		}
		return LoadEntry{}, nil
	case EntryScan:
		if reasonCode != "" || note != "" {
			return LoadEntry{}, fmt.Errorf("invalid: reasonCode and note only apply to a MANUAL entry")
		}
		return LoadEntry{Method: EntryScan}, nil
	case EntryManual:
		if reasonCode == "" {
			return LoadEntry{}, fmt.Errorf("invalid: reasonCode is required when the order ID is typed instead of scanned")
		}
		if !ValidManualReason(reasonCode) {
			return LoadEntry{}, fmt.Errorf("invalid: reasonCode must be DAMAGED_LABEL, UNREADABLE, NO_CAMERA or OTHER")
		}
		if utf8.RuneCountInString(note) > MaxManualNoteLength {
			return LoadEntry{}, fmt.Errorf("invalid: note too long (max %d characters)", MaxManualNoteLength)
		}
		return LoadEntry{Method: EntryManual, ReasonCode: reasonCode, Note: note}, nil
	}
	return LoadEntry{}, fmt.Errorf("invalid: entryMethod must be SCAN or MANUAL")
}
