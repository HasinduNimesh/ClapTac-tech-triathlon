package service

import (
	"fmt"
	"math"
	"strings"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
)

const maxOrderLines = 200

var lineSources = map[string]bool{"form": true, "text_helper": true, "habit_helper": true}

// priceLines turns the client's (product, packs) pairs into order lines using the catalog's own pack weight and
// volume, and sets the request's units, weight, volume and temperature from them. A client therefore cannot send
// totals that disagree with the items, and the planner's vehicle weight and volume checks see the real load.
func (s Service) priceLines(bearer, brand string, req *domain.CreateRequest) ([]domain.OrderLine, error) {
	if len(req.Lines) > maxOrderLines {
		return nil, fmt.Errorf("%w: at most %d lines", ErrInvalid, maxOrderLines)
	}
	source := strings.TrimSpace(req.LinesSource)
	if source == "" {
		source = "form"
	}
	if !lineSources[source] {
		return nil, fmt.Errorf("%w: linesSource", ErrInvalid)
	}
	// The same product twice is one line; quantities add up.
	order := []string{}
	packs := map[string]int{}
	for _, l := range req.Lines {
		id := strings.TrimSpace(l.ProductID)
		if id == "" || l.PackQty <= 0 {
			return nil, fmt.Errorf("%w: every line needs a product and a quantity above zero", ErrInvalid)
		}
		if _, seen := packs[id]; !seen {
			order = append(order, id)
		}
		packs[id] += l.PackQty
		if packs[id] > 999 {
			return nil, fmt.Errorf("%w: at most 999 packs of one product", ErrInvalid)
		}
	}
	if s.Products == nil {
		return nil, fmt.Errorf("%w: product catalog", ErrUnavailable)
	}
	found, err := s.Products.Products(order, bearer)
	if err != nil {
		return nil, fmt.Errorf("%w: product catalog", ErrUnavailable)
	}
	catalog := make(map[string]domain.Product, len(found))
	for _, p := range found {
		catalog[p.ID] = p
	}
	var (
		lines          []domain.OrderLine
		units          int
		weight, volume float64
		temperature    string
	)
	for i, id := range order {
		p, ok := catalog[id]
		if !ok {
			return nil, fmt.Errorf("%w: %s is not an orderable product", ErrInvalid, id)
		}
		if brand != "" && !strings.EqualFold(p.Brand, brand) {
			return nil, fmt.Errorf("%w: %s is not stocked by this outlet's brand", ErrInvalid, id)
		}
		if temperature == "" {
			temperature = p.Temperature
		} else if temperature != p.Temperature {
			return nil, fmt.Errorf("%w: chilled and ambient goods need separate orders", ErrInvalid)
		}
		qty := packs[id]
		lineWeight := round(float64(qty)*p.PackWeightKg, 3)
		lineVolume := round(float64(qty)*p.PackVolumeM3, 4)
		lines = append(lines, domain.OrderLine{
			LineNo: i + 1, ProductID: p.ID, ProductName: p.Name, Pack: p.Pack, UnitsPerPack: p.UnitsPerPack,
			PackQty: qty, WeightKg: lineWeight, VolumeM3: lineVolume, Source: source,
		})
		units += qty
		weight += lineWeight
		volume += lineVolume
	}
	if requested := strings.ToLower(string(req.TemperatureRequirement)); requested != "" && requested != temperature {
		return nil, fmt.Errorf("%w: temperatureRequirement does not match the items", ErrInvalid)
	}
	req.OrderUnits = units
	req.OrderWeightKg = round(weight, 3)
	// Orders keep volume to three decimals; round up so a small order never understates the space it needs.
	req.OrderVolumeM3 = math.Ceil(round(volume, 4)*1000) / 1000
	req.TemperatureRequirement = domain.Temperature(temperature)
	return lines, nil
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}
