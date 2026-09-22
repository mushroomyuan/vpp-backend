// Package grpc implements forecastpb.ForecastService: the read-only
// GetLatestPrediction and QueryForecastHistory RPCs. There is no write
// RPC (design plan §3). v1 does not run grpc-gateway; clients speak
// native gRPC on :5007. The process registers this Server in place of
// Unimplemented once application.Application is wired at the composition root.
package grpc
