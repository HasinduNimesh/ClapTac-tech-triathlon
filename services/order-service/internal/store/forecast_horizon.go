package store

import (
	"math"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
)

const (
	// ForecastHorizonWeeks is how far ahead the demand forecast looks: ten
	// calendar weeks (70 days), starting with the current Monday-based week.
	ForecastHorizonWeeks = 10
	ForecastHorizonDays  = 7 * ForecastHorizonWeeks
	// forecastHistoryWeeks is the number of complete prior weeks averaged into
	// the baseline.
	forecastHistoryWeeks = 4
	// forecastVersion identifies the projection method. v2 extends the v1
	// four-week mean from four to ten projected weeks and attaches a range that
	// widens with distance.
	forecastVersion = "confirmed_order_mean_v2"
	// The baseline is a stationary mean: every projected week carries the same
	// estimate because no trend or seasonality is modelled. Its likely range
	// therefore widens with distance, forecastRangeBasePercent in the first
	// week plus forecastRangeWidenPercent for each week further out. These are
	// planning heuristics, not a calibrated interval.
	forecastRangeBasePercent  = 10.0
	forecastRangeWidenPercent = 2.0
	forecastMethod            = "mean of confirmed order volume over the previous four complete weeks, held flat across the next ten weeks (no trend or seasonality is modelled); the likely range widens from 10% in the first week by 2 points for each week further out; estimates only"
)

// forecastRangePercent is the half-width, in percent, of the likely range for a
// projected week. horizonWeek is 1 for the first projected week.
func forecastRangePercent(horizonWeek int) float64 {
	if horizonWeek < 1 {
		horizonWeek = 1
	}
	return forecastRangeBasePercent + forecastRangeWidenPercent*float64(horizonWeek-1)
}

type forecastGroup struct {
	depot, brand     string
	chilled, ambient int
	weight, volume   float64
}

// projectWeekly spreads the history-window totals of each depot and brand
// evenly over the history weeks and repeats that baseline for every week of
// the horizon. The output is ordered by week, then by the order of groups.
func projectWeekly(weekStart time.Time, groups []forecastGroup) []domain.ForecastBucket {
	out := make([]domain.ForecastBucket, 0, ForecastHorizonWeeks*len(groups))
	for w := 0; w < ForecastHorizonWeeks; w++ {
		d := weekStart.AddDate(0, 0, 7*w)
		for _, g := range groups {
			out = append(out, domain.ForecastBucket{
				WeekStarting:      d.Format("2006-01-02"),
				HorizonWeek:       w + 1,
				RangePercent:      forecastRangePercent(w + 1),
				Depot:             g.depot,
				Brand:             g.brand,
				ChilledOrders:     int(math.Round(float64(g.chilled) / forecastHistoryWeeks)),
				AmbientOrders:     int(math.Round(float64(g.ambient) / forecastHistoryWeeks)),
				EstimatedWeightKg: math.Round(g.weight/forecastHistoryWeeks*100) / 100,
				EstimatedVolumeM3: math.Round(g.volume/forecastHistoryWeeks*1000) / 1000,
				Estimate:          true,
			})
		}
	}
	return out
}
