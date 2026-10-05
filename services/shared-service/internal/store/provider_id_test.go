package store

import "testing"

func TestValidProviderMessageID(t *testing.T) {
	valid := []string{
		"SM0123456789abcdef0123456789abcdef", // a Twilio message SID
		"cg:req_Hr93eRmiqYu0Ed2nWsSdaQ",      // our cellular gateway's request id
		"cg:abcd",
	}
	for _, id := range valid {
		if !validProviderMessageID(id) {
			t.Errorf("%q should be accepted", id)
		}
	}
	invalid := []string{
		"",
		"SM123",                                // too short for a Twilio SID
		"XM0123456789abcdef0123456789abcdef",   // wrong Twilio prefix
		"cg:",                                  // nothing after the prefix
		"cg:abc",                               // too short
		"cg:req Hr93",                          // a space
		"cg:req_Hr93eRmiqYu0Ed2nWsSdaQ'; DROP", // punctuation
		"CG:req_Hr93eRmiqYu0Ed2nWsSdaQ",        // the prefix is lower case
		"cg:" + string(make([]byte, 80)),       // too long
		"cg:req_Hr93eRmiqYu0Ed2nWsSdaQ\n",      // a trailing newline
	}
	for _, id := range invalid {
		if validProviderMessageID(id) {
			t.Errorf("%q should be refused", id)
		}
	}
}
