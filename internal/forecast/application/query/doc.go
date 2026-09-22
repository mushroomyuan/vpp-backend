// Package query holds the read use-cases behind ForecastService:
// GetLatestPrediction (cache then Postgres, SelectNextPoint) and
// QueryForecastHistory (Postgres window).
package query
