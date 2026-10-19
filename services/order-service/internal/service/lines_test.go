package service

import (
	"errors"
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/cutoff"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/store"
)

type testProducts struct {
	items []domain.Product
	err   error
	asked []string
}

func (p *testProducts) Range(string) ([]domain.Product, error) { return p.items, p.err }

func (p *testProducts) Products(ids []string, _ string) ([]domain.Product, error) {
	p.asked = ids
	return p.items, p.err
}

var catalog = []domain.Product{
	{ID: "FR-MILK-1L", Brand: "Fresh", Name: "Fresh milk 1 L", Pack: "crate", UnitsPerPack: 12, PackWeightKg: 12.6, PackVolumeM3: 0.021, Temperature: "chilled"},
	{ID: "FR-CURD-500G", Brand: "Fresh", Name: "Curd 500 g", Pack: "tray", UnitsPerPack: 8, PackWeightKg: 4.2, PackVolumeM3: 0.0114, Temperature: "chilled"},
	{ID: "FR-RICE-5KG", Brand: "Fresh", Name: "Samba rice 5 kg", Pack: "bag", UnitsPerPack: 1, PackWeightKg: 5.1, PackVolumeM3: 0.009, Temperature: "ambient"},
	{ID: "ST-SHIRT-M", Brand: "Style", Name: "Shirt M", Pack: "bale", UnitsPerPack: 10, PackWeightKg: 3, PackVolumeM3: 0.03, Temperature: "ambient"},
}

func linesService(products *testProducts) (Service, *authorization.Profile) {
	loc, _ := time.LoadLocation(cutoff.Zone)
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, loc)
	s := Service{Repo: store.NewMemory(), Outlets: testOutlets{}, Cutoff: cutoff.Load(nil), Now: func() time.Time { return now }, Products: products}
	return s, &authorization.Profile{UserID: "USR001", Roles: []string{authorization.RoleStoreManager}, OutletIDs: []string{"OUT001"}}
}

func place(s Service, p *authorization.Profile, req domain.CreateRequest) (domain.Order, error) {
	req.RequestedDeliveryDate = "2026-10-07"
	return s.Create(p, "Bearer t", req)
}

func TestTotalsAreComputedFromTheLinesNotTheClient(t *testing.T) {
	s, profile := linesService(&testProducts{items: catalog})
	created, err := place(s, profile, domain.CreateRequest{
		// The client's own totals and temperature are ignored or checked against the items.
		OrderUnits: 1, OrderWeightKg: 1, OrderVolumeM3: 1,
		Lines: []domain.LineRequest{{ProductID: "FR-MILK-1L", PackQty: 3}, {ProductID: "FR-CURD-500G", PackQty: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.OrderUnits != 5 || created.OrderWeightKg != 46.2 || created.OrderVolumeM3 != 0.086 || created.TemperatureRequirement != domain.TempChilled {
		t.Fatalf("totals must be 5 packs, 46.2 kg, 0.086 m3, chilled: %+v", created)
	}
	if len(created.Lines) != 2 || created.Lines[0].LineNo != 1 || created.Lines[0].ProductName != "Fresh milk 1 L" || created.Lines[0].Pack != "crate" || created.Lines[0].WeightKg != 37.8 || created.Lines[1].Source != "form" {
		t.Fatalf("lines: %+v", created.Lines)
	}
	got, err := s.Get(profile, created.ID)
	if err != nil || len(got.Lines) != 2 {
		t.Fatalf("lines must come back with the order: %+v %v", got, err)
	}
}

func TestVolumeIsRoundedUpSoSmallOrdersNeverUnderstateSpace(t *testing.T) {
	s, profile := linesService(&testProducts{items: catalog})
	created, err := place(s, profile, domain.CreateRequest{Lines: []domain.LineRequest{{ProductID: "FR-RICE-5KG", PackQty: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if created.OrderVolumeM3 != 0.009 || created.OrderWeightKg != 5.1 {
		t.Fatalf("%+v", created)
	}
	created, err = place(s, profile, domain.CreateRequest{Lines: []domain.LineRequest{{ProductID: "FR-CURD-500G", PackQty: 1}}})
	if err != nil || created.OrderVolumeM3 != 0.012 {
		t.Fatalf("0.0114 m3 must round up to 0.012, got %+v %v", created, err)
	}
}

func TestSameProductTwiceIsOneLine(t *testing.T) {
	products := &testProducts{items: catalog}
	s, profile := linesService(products)
	created, err := place(s, profile, domain.CreateRequest{Lines: []domain.LineRequest{{ProductID: "FR-RICE-5KG", PackQty: 2}, {ProductID: "FR-RICE-5KG", PackQty: 3}}})
	if err != nil || len(created.Lines) != 1 || created.Lines[0].PackQty != 5 || created.OrderUnits != 5 {
		t.Fatalf("%+v %v", created, err)
	}
	if len(products.asked) != 1 {
		t.Fatalf("the catalog is asked once per product: %v", products.asked)
	}
}

func TestBadLinesAreRefused(t *testing.T) {
	cases := map[string]domain.CreateRequest{
		"unknown product":         {Lines: []domain.LineRequest{{ProductID: "FR-NOPE", PackQty: 1}}},
		"other brand":             {Lines: []domain.LineRequest{{ProductID: "ST-SHIRT-M", PackQty: 1}}},
		"mixed temperatures":      {Lines: []domain.LineRequest{{ProductID: "FR-MILK-1L", PackQty: 1}, {ProductID: "FR-RICE-5KG", PackQty: 1}}},
		"zero quantity":           {Lines: []domain.LineRequest{{ProductID: "FR-RICE-5KG", PackQty: 0}}},
		"negative quantity":       {Lines: []domain.LineRequest{{ProductID: "FR-RICE-5KG", PackQty: -2}}},
		"too many of one product": {Lines: []domain.LineRequest{{ProductID: "FR-RICE-5KG", PackQty: 600}, {ProductID: "FR-RICE-5KG", PackQty: 600}}},
		"blank product":           {Lines: []domain.LineRequest{{ProductID: " ", PackQty: 1}}},
		"wrong temperature":       {TemperatureRequirement: domain.TempAmbient, Lines: []domain.LineRequest{{ProductID: "FR-MILK-1L", PackQty: 1}}},
		"unknown source":          {LinesSource: "hacker", Lines: []domain.LineRequest{{ProductID: "FR-RICE-5KG", PackQty: 1}}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			s, profile := linesService(&testProducts{items: catalog})
			if _, err := place(s, profile, req); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want invalid, got %v", err)
			}
		})
	}
	s, profile := linesService(&testProducts{items: catalog})
	many := make([]domain.LineRequest, 201)
	for i := range many {
		many[i] = domain.LineRequest{ProductID: "FR-RICE-5KG", PackQty: 1}
	}
	if _, err := place(s, profile, domain.CreateRequest{Lines: many}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("201 lines: %v", err)
	}
}

func TestOrdersWithLinesNeedTheCatalogButOrdersWithoutDoNot(t *testing.T) {
	s, profile := linesService(&testProducts{err: errors.New("down")})
	if _, err := place(s, profile, domain.CreateRequest{Lines: []domain.LineRequest{{ProductID: "FR-RICE-5KG", PackQty: 1}}}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("catalog down must be unavailable, not an order with guessed weights: %v", err)
	}
	s.Products = nil
	if _, err := place(s, profile, domain.CreateRequest{Lines: []domain.LineRequest{{ProductID: "FR-RICE-5KG", PackQty: 1}}}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no catalog configured: %v", err)
	}
	plain, err := place(s, profile, domain.CreateRequest{OrderUnits: 4, OrderWeightKg: 20, OrderVolumeM3: 0.5, TemperatureRequirement: domain.TempAmbient})
	if err != nil || len(plain.Lines) != 0 || plain.OrderUnits != 4 {
		t.Fatalf("an order with totals only still works: %+v %v", plain, err)
	}
	if _, err := place(s, profile, domain.CreateRequest{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("an empty order is still invalid: %v", err)
	}
}

func TestHelperSourcesAreKept(t *testing.T) {
	s, profile := linesService(&testProducts{items: catalog})
	created, err := place(s, profile, domain.CreateRequest{LinesSource: "text_helper", Lines: []domain.LineRequest{{ProductID: "FR-RICE-5KG", PackQty: 1}}})
	if err != nil || created.Lines[0].Source != "text_helper" {
		t.Fatalf("%+v %v", created, err)
	}
}
