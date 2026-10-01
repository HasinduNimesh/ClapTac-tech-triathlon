package domain

import (
	"encoding/json"
	"testing"
)

func TestResultJSONUsesClientContractFieldNames(t *testing.T) {
	encoded, err := json.Marshal(Result{
		Valid:      false,
		ReasonCode: ReasonDepotMismatch,
		Details:    map[string]any{"vehicleDepot": "DEPOT_SOUTH"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got["valid"] != false || got["reasonCode"] != ReasonDepotMismatch {
		t.Fatalf("constraint result does not match the planning API contract: %s", encoded)
	}
	if _, ok := got["ReasonCode"]; ok {
		t.Fatalf("constraint result leaked Go field names: %s", encoded)
	}
}
