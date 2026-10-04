package service

import (
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
)

func floatPtr(v float64) *float64 { return &v }

func TestReceiptTemperatureIsKeptForChilledOrders(t *testing.T) {
	s, repo, _, p := receiptService("DELIVERED")
	_, _ = repo.Create(domain.Order{ID: "order-chilled", OrderRef: "ORD000002", OutletID: "OUT034", OrderUnits: 10, TemperatureRequirement: domain.TempChilled, Status: domain.StatusConfirmed})
	r, _, created, err := s.ConfirmReceipt(p, "order-chilled", domain.ReceiptConfirmation{ReceivedUnits: 10, ReceivedTemperatureC: floatPtr(4.5)})
	if err != nil || !created || r.ReceivedTemperatureC == nil || *r.ReceivedTemperatureC != 4.5 {
		t.Fatalf("chilled receipt with a reading: %+v %v", r, err)
	}
}

func TestReceiptTemperatureIsOptionalAndChecked(t *testing.T) {
	s, repo, _, p := receiptService("DELIVERED")
	_, _ = repo.Create(domain.Order{ID: "order-chilled", OrderRef: "ORD000002", OutletID: "OUT034", OrderUnits: 10, TemperatureRequirement: domain.TempChilled, Status: domain.StatusConfirmed})
	if _, _, _, err := s.ConfirmReceipt(p, "order-chilled", domain.ReceiptConfirmation{ReceivedUnits: 10, ReceivedTemperatureC: floatPtr(75)}); err == nil {
		t.Fatal("an implausible reading was accepted")
	}
	if _, _, _, err := s.ConfirmReceipt(p, "order-1", domain.ReceiptConfirmation{ReceivedUnits: 20, ReceivedTemperatureC: floatPtr(4)}); err == nil {
		t.Fatal("a reading on an ambient order was accepted")
	}
	r, _, _, err := s.ConfirmReceipt(p, "order-chilled", domain.ReceiptConfirmation{ReceivedUnits: 10})
	if err != nil || r.ReceivedTemperatureC != nil {
		t.Fatalf("a chilled receipt without a reading must still confirm: %+v %v", r, err)
	}
}
