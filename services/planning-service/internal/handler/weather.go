package handler

import (
	"context"
	"net/http"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/weather"
)

type WeatherForecaster interface {
	ForDate(ctx context.Context, date string) (weather.Forecast, error)
}

// weatherForecast lists every delivery district with its rain forecast and any suggested risk.
// Without a forecaster the districts are still listed, so the maps can place recorded risks.
func (h Handler) weatherForecast(w http.ResponseWriter, r *http.Request) {
	forecaster := h.Weather
	if forecaster == nil {
		forecaster = &weather.Client{}
	}
	forecast, err := forecaster.ForDate(r.Context(), r.URL.Query().Get("date"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, forecast)
}
