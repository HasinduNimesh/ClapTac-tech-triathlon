package domain

import "math"

const (
	LatenessModelVersion       = "depot_brand_temperature_90d_late_rate_laplace_v2"
	LatenessCalibrationVersion = "rolling_90d_depot_brand_temperature_brier_v2"
	LatenessHistoryDays        = 90
	LatenessMinSamples         = 10
	LatenessBacktestDays       = 180
	ArrivalRangeModelVersion   = "planned_eta_residual_p10_p90_90d_v1"
	ArrivalRangeMinSamples     = 20
	ArrivalRangeMinTraining    = 10
	ArrivalRangeMinHoldout     = 10
)

type LatenessProbability struct {
	Depot                  string   `json:"depot"`
	Brand                  string   `json:"brand"`
	TemperatureRequirement string   `json:"temperatureRequirement"`
	SampleCount            int      `json:"sampleCount"`
	LateCount              int      `json:"lateCount"`
	Probability            *float64 `json:"probability,omitempty"`
	Status                 string   `json:"status"`
	ModelVersion           string   `json:"modelVersion"`
	HistoryDays            int      `json:"historyDays"`
	Definition             string   `json:"definition"`
	CalibrationSampleCount int      `json:"calibrationSampleCount"`
	BrierScore             *float64 `json:"brierScore,omitempty"`
	CalibrationStatus      string   `json:"calibrationStatus"`
	CalibrationVersion     string   `json:"calibrationVersion"`
	ArrivalRangeStatus     string   `json:"arrivalRangeStatus"`
	ArrivalRangeVersion    string   `json:"arrivalRangeVersion"`
	ArrivalRangeSamples    int      `json:"arrivalRangeSamples"`
	ArrivalRangeHoldouts   int      `json:"arrivalRangeHoldouts"`
	ArrivalRangeCoverage   *float64 `json:"arrivalRangeCoverage,omitempty"`
	ArrivalOffsetP10       *float64 `json:"arrivalOffsetP10Minutes,omitempty"`
	ArrivalOffsetP90       *float64 `json:"arrivalOffsetP90Minutes,omitempty"`
}

func ApplyArrivalRange(result LatenessProbability, samples, holdouts int, coverage, p10, p90 *float64) LatenessProbability {
	result.ArrivalRangeStatus = "INSUFFICIENT_HISTORY"
	result.ArrivalRangeVersion = ArrivalRangeModelVersion
	result.ArrivalRangeSamples = samples
	result.ArrivalRangeHoldouts = holdouts
	if coverage != nil && validProbability(*coverage) {
		x := math.Round(*coverage*10000) / 10000
		result.ArrivalRangeCoverage = &x
	}
	validBounds := p10 != nil && p90 != nil && !math.IsNaN(*p10) && !math.IsNaN(*p90) && !math.IsInf(*p10, 0) && !math.IsInf(*p90, 0) && *p10 <= *p90
	if samples < ArrivalRangeMinSamples || !validBounds {
		return result
	}
	if holdouts < ArrivalRangeMinHoldout || result.ArrivalRangeCoverage == nil {
		result.ArrivalRangeStatus = "INSUFFICIENT_HOLDOUT"
		return result
	}
	if *result.ArrivalRangeCoverage < 0.70 || *result.ArrivalRangeCoverage > 0.90 {
		result.ArrivalRangeStatus = "POOR_COVERAGE"
		return result
	}
	lower, upper := math.Round(*p10*10)/10, math.Round(*p90*10)/10
	result.ArrivalOffsetP10 = &lower
	result.ArrivalOffsetP90 = &upper
	result.ArrivalRangeStatus = "CALIBRATED"
	return result
}

func validProbability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func EstimateLatenessProbability(depot, brand string, samples, late int) LatenessProbability {
	result := LatenessProbability{
		Depot: depot, Brand: brand, SampleCount: samples, LateCount: late,
		Status: "INSUFFICIENT_HISTORY", ModelVersion: LatenessModelVersion,
		HistoryDays:        LatenessHistoryDays,
		Definition:         "Observed arrivals after the delivery-window close in this depot/brand/temperature segment; Laplace-smoothed rate.",
		CalibrationStatus:  "INSUFFICIENT_HOLDOUT",
		ArrivalRangeStatus: "INSUFFICIENT_HISTORY", ArrivalRangeVersion: ArrivalRangeModelVersion,
	}
	if samples < LatenessMinSamples || late < 0 || late > samples {
		return result
	}
	probability := math.Round((float64(late+1)/float64(samples+2))*10000) / 10000
	result.Probability = &probability
	result.Status = "ESTIMATED"
	return result
}

func ApplyLatenessCalibration(result LatenessProbability, samples int, brier *float64) LatenessProbability {
	result.CalibrationSampleCount = samples
	result.CalibrationStatus = "INSUFFICIENT_HOLDOUT"
	result.CalibrationVersion = LatenessCalibrationVersion
	if samples < LatenessMinSamples || brier == nil || math.IsNaN(*brier) || math.IsInf(*brier, 0) || *brier < 0 || *brier > 1 {
		return result
	}
	score := math.Round(*brier*10000) / 10000
	result.BrierScore = &score
	result.CalibrationStatus = "EVALUATED"
	return result
}
