package service

import (
    "testing"

    "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/allocate"
    "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
)

func TestDispatcherDeferralRequestStaysOutOfAutomaticAllocation(t *testing.T) {
    world := allocate.Input{Orders: []domain.Order{
        {ID:"retry",SourceSystem:"delivery-reattempt"},
        {ID:"defer",SourceSystem:"delivery-deferral-request"},
    }}
    candidates, pending := holdDispatcherDeferrals(world)
    if len(candidates.Orders) != 1 || candidates.Orders[0].ID != "retry" ||
        len(pending) != 1 || pending[0].OrderID != "defer" || pending[0].ReasonCode != domain.ReasonManualDeferral {
        t.Fatalf("candidates=%+v pending=%+v", candidates.Orders, pending)
    }
}
