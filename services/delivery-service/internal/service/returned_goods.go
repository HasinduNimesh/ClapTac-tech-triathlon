package service

import (
    "context"
    "fmt"
    "strings"
    "unicode/utf8"

    "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func validReturnDetails(details domain.ReturnDetails) bool {
    goods := strings.TrimSpace(details.Goods)
    return goods != "" && utf8.RuneCountInString(goods) <= 200 && details.Units > 0 &&
        (details.Resolution == "NEXT_RUN" || details.Resolution == "REQUEST_DEFERRAL")
}

func (s Service) finishReturnedGoods(ctx context.Context, stopID string) error {
    item, err := s.Repo.ReturnedGoods(ctx, stopID)
    if err != nil { return err }
    if item == nil { return fmt.Errorf("conflict: returned goods missing") }
    if item.FollowupOrderID == "" {
        tripDate, err := s.Repo.ReturnTripDate(ctx, item.RunID)
        if err != nil { return err }
        id, ref, date, err := s.Peers.CreateDeliveryFollowup(ctx, item.OrderID, stopID, tripDate, item.Units, item.Resolution)
        if err != nil { return err }
        if err := s.Repo.LinkReturnFollowup(ctx, stopID, id, ref, date); err != nil { return err }
        item.FollowupOrderID, item.FollowupOrderRef, item.FollowupDate = id, ref, date
    }
    if err := s.Repo.EnsureReturnDispatcherMessage(ctx, stopID); err != nil { return err }
    // The store notice is best effort: a notice that cannot be queued must not
    // turn the driver's recorded take-back into an error.
    if err := s.Peers.QueueReturnedGoodsNotice(ctx, *item); err != nil {
        s.Peers.Logger.Error("returned_goods_notice_failed", "stop_id", stopID, "error", err)
    }
    return nil
}
