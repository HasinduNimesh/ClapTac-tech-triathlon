package store

import "math"

const (
	forecastDriftVersion          = "weekly_order_shift_v1"
	forecastDriftMinOrders        = 10
	forecastDriftThreshold        = 50.0
	forecastBacktestVersion       = "prior_four_week_order_count_ape_v1"
	serviceTimeBacktestVersion    = "configured_service_time_mae_v1"
	serviceTimeEvaluationMinStops = 10
)

func classifyForecastDrift(previous, recent int) (string, *float64) {
	if previous < 0 || recent < 0 || previous+recent < forecastDriftMinOrders {
		return "INSUFFICIENT_HISTORY", nil
	}
	if previous == 0 {
		return "NEW_DEMAND", nil
	}
	change := math.Round((float64(recent-previous)/float64(previous))*10000) / 100
	if math.Abs(change) >= forecastDriftThreshold {
		return "SHIFTED", &change
	}
	return "STABLE", &change
}

func forecastAPE(predicted, actual int) *float64 {
	if predicted < 0 || actual < forecastDriftMinOrders {
		return nil
	}
	errorPercent := math.Round((math.Abs(float64(predicted-actual))/float64(actual))*10000) / 100
	return &errorPercent
}
