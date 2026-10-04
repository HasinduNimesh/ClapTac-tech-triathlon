package service

import (
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func TestAssessCheckout(t *testing.T) {
	orders := []domain.LoadingOrder{
		{OrderID: "one", LoadingStatus: "loaded"},
		{OrderID: "two", LoadingStatus: "loaded"},
	}
	missing, err := assessCheckout(orders, []string{"one"})
	if err != nil || len(missing) != 1 || missing[0] != "two" { t.Fatalf("missing stop: %v %v", missing, err) }
	missing, err = assessCheckout(orders, []string{"one", "two"})
	if err != nil || len(missing) != 0 { t.Fatalf("all onboard: %v %v", missing, err) }
	orders[1].LoadingStatus = "shortfall"
	missing, err = assessCheckout(orders, []string{"one", "two"})
	if err != nil || len(missing) != 0 { t.Fatalf("a cleared partial load the driver confirms is not missing: %v %v", missing, err) }
	missing, err = assessCheckout(orders, []string{"one"})
	if err != nil || len(missing) != 1 || missing[0] != "two" { t.Fatalf("an unconfirmed partial load is missing: %v %v", missing, err) }
	orders[1].LoadingStatus = "pending"
	missing, err = assessCheckout(orders, []string{"one", "two"})
	if err != nil || len(missing) != 1 || missing[0] != "two" { t.Fatalf("a line still pending is missing: %v %v", missing, err) }
	if _, err = assessCheckout(orders, []string{"one", "unknown"}); err == nil { t.Fatal("unknown order accepted") }
	if _, err = assessCheckout(orders, []string{"one", "one"}); err == nil { t.Fatal("duplicate order accepted") }
}
