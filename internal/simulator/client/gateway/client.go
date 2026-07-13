package gatewayclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/simulator/domain"
)

// Config for Gateway HTTP ingest.
type Config struct {
	BaseURL string // e.g. http://127.0.0.1:8083
	Timeout time.Duration
}

// Client posts telemetry to Gateway.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

type ingestRequest struct {
	ExternalSystem string         `json:"external_system"`
	ExternalID     string         `json:"external_id"`
	Timestamp      string         `json:"timestamp,omitempty"`
	Metrics        []ingestMetric `json:"metrics"`
}

type ingestMetric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

func New(cfg Config) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("gateway client: base url is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL: base,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

const ExternalSystem = "simulator"

// IngestTelemetry pushes one CU snapshot to Gateway.
func (c *Client) IngestTelemetry(
	ctx context.Context,
	tenantID, externalID string,
	points []domain.PointValue,
	ts time.Time,
) error {
	if len(points) == 0 {
		return nil
	}
	metrics := make([]ingestMetric, 0, len(points))
	for _, p := range points {
		metrics = append(metrics, ingestMetric{Name: p.PointKey, Value: p.Value})
	}
	body, err := json.Marshal(ingestRequest{
		ExternalSystem: ExternalSystem,
		ExternalID:     externalID,
		Timestamp:      ts.UTC().Format(time.RFC3339Nano),
		Metrics:        metrics,
	})
	if err != nil {
		return fmt.Errorf("marshal ingest: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/tenants/%s/telemetry:ingest", c.baseURL, tenantID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build ingest request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ingest telemetry: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ingest telemetry status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}
