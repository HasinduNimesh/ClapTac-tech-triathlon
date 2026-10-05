package store

import (
	"context"
	"regexp"
	"strconv"
)

// Product is one orderable item. A pack is what moves through ordering, loading and delivery, so the weight
// and volume are per pack.
type Product struct {
	ID           string  `json:"id"`
	Brand        string  `json:"brand"`
	Family       string  `json:"family"`
	Name         string  `json:"name"`
	Size         string  `json:"size"`
	Pack         string  `json:"pack"`
	UnitsPerPack int     `json:"unitsPerPack"`
	PackWeightKg float64 `json:"packWeightKg"`
	PackVolumeM3 float64 `json:"packVolumeM3"`
	Temperature  string  `json:"temperature"`
	Season       string  `json:"season,omitempty"`
}

var productID = regexp.MustCompile(`^(FR|ST|TC)-[A-Z0-9-]{2,40}$`)

// ValidProductID reports whether id has the shape of a catalog product id.
func ValidProductID(id string) bool { return productID.MatchString(id) }

const productSelect = `SELECT p.id, p.brand, p.family, p.name, p.size, p.pack_name, p.units_per_pack,
	p.pack_weight_kg::float8, p.pack_volume_m3::float8, p.temperature, COALESCE(st.season, '')
	FROM shared.products p LEFT JOIN shared.product_style st ON st.product_id = p.id`

// Products returns active products. ids limits the result to those products; otherwise outletID limits it to the
// products that outlet lists, and brand to one brand. Results are ordered by brand, family and name.
func (s Store) Products(ctx context.Context, ids []string, outletID, brand string) ([]Product, error) {
	query := productSelect + ` WHERE p.status = 'active'`
	args := []any{}
	if len(ids) > 0 {
		args = append(args, ids)
		query += ` AND p.id = ANY($1)`
	} else {
		if outletID != "" {
			args = append(args, outletID)
			query += ` AND EXISTS (SELECT 1 FROM shared.outlet_products op WHERE op.product_id = p.id AND op.outlet_id = $` + strconv.Itoa(len(args)) + ` AND op.listed)`
		}
		if brand != "" {
			args = append(args, brand)
			query += ` AND p.brand = $` + strconv.Itoa(len(args))
		}
	}
	rows, err := s.Pool.Query(ctx, query+` ORDER BY p.brand, p.family, p.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Product{}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Brand, &p.Family, &p.Name, &p.Size, &p.Pack, &p.UnitsPerPack, &p.PackWeightKg, &p.PackVolumeM3, &p.Temperature, &p.Season); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
