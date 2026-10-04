package service

import (
	"context"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
)

// notifyStoreOfShortfall tells the affected outlet what will arrive once the
// dispatcher has decided. The event key is stable per issue and decision, so
// re-recording the same decision never sends a second notice. Failures are
// logged and never block the dispatcher's decision.
func (s Service) notifyStoreOfShortfall(ctx context.Context, load domain.OrderLoad, iss domain.Issue, decision string) {
	if iss.AffectedUnits < 1 {
		return
	}
	outletID, orderRef := load.OutletID, load.OrderRef
	if outletID == "" || orderRef == "" {
		d, err := s.orderDetails(ctx, []string{load.OrderID})
		if err != nil {
			if s.Peers.Logger != nil {
				s.Peers.Logger.Error("shortfall_notice_order_lookup_failed", "issue", iss.ID, "error", err)
			}
			return
		}
		outletID, orderRef = d[load.OrderID].OutletID, d[load.OrderID].OrderRef
	}
	if outletID == "" || orderRef == "" {
		return
	}
	if err := s.Peers.QueueShortfallNotice(ctx, "shortfall:"+iss.ID+":"+decision, outletID, orderRef, decision, iss.AffectedUnits); err != nil && s.Peers.Logger != nil {
		s.Peers.Logger.Error("shortfall_notice_failed", "issue", iss.ID, "error", err)
	}
}
