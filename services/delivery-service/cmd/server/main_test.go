package main

import "testing"

func TestDeliveryProofRetentionConfigDefaultsToDisabledDraft(t *testing.T) {
	enabled, days, err := deliveryProofRetentionConfig("", "")
	if err != nil || enabled || days != 180 {
		t.Fatalf("config=(%t,%d,%v), want (false,180,nil)", enabled, days, err)
	}
}

func TestDeliveryProofRetentionConfigValidation(t *testing.T) {
	for _, input := range []struct {
		enabled string
		days    string
		want    bool
	}{
		{enabled: "TRUE", days: "1", want: true},
		{enabled: "false", days: "3650", want: false},
	} {
		enabled, days, err := deliveryProofRetentionConfig(input.enabled, input.days)
		if err != nil || enabled != input.want {
			t.Fatalf("config(%q,%q)=(%t,%d,%v)", input.enabled, input.days, enabled, days, err)
		}
	}
	for _, input := range []struct{ enabled, days string }{
		{enabled: "sometimes", days: "180"},
		{enabled: "true", days: "0"},
		{enabled: "false", days: "3651"},
		{enabled: "true", days: "many"},
	} {
		if _, _, err := deliveryProofRetentionConfig(input.enabled, input.days); err == nil {
			t.Fatalf("config(%q,%q) unexpectedly accepted", input.enabled, input.days)
		}
	}
}
