package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

const ExternalSystem = "simulator"

// Config holds the HTTP endpoint of the vpp-simulator service.
type Config struct {
	// Addr is the base URL, e.g. "http://127.0.0.1:8084".
	// Empty → client must not be constructed (use Router with nil simulator).
	Addr    string
	Timeout time.Duration
}

// Client delivers control commands to vpp-simulator over HTTP.
// It implements port.EMSClient so Gateway can treat Simulator like any other
// external system (EMS, IoT platform, single-device CU, etc.).
type Client struct {
	baseURL    string
	httpClient *http.Client
}

var _ port.EMSClient = (*Client)(nil)

type commandRequest struct {
	CommandID  string  `json:"command_id"`
	ExternalID string  `json:"external_id"`
	PointKey   string  `json:"point_key"`
	Value      float64 `json:"value"`
}

type commandResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message,omitempty"`
}

// NewClient builds an HTTP client targeting the simulator command API.
func NewClient(cfg Config) (*Client, error) {
	addr := strings.TrimRight(strings.TrimSpace(cfg.Addr), "/")
	if addr == "" {
		return nil, fmt.Errorf("simulator: addr is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	logrus.Infof("simulator outbound: targeting %s", addr)
	return &Client{
		baseURL: addr,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// SendCommand POSTs to Simulator POST /api/v1/commands.
func (c *Client) SendCommand(
	ctx context.Context,
	commandID, externalSystem, externalID, command string,
	value float64,
) error {
	body, err := json.Marshal(commandRequest{
		CommandID:  commandID,
		ExternalID: externalID,
		PointKey:   command,
		Value:      value,
	})
	if err != nil {
		return fmt.Errorf("simulator: marshal command: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/commands", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("simulator: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("simulator: send command: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("simulator: command rejected status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed commandResponse
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &parsed)
		if !parsed.Accepted && parsed.Message != "" {
			return fmt.Errorf("simulator: command not accepted: %s", parsed.Message)
		}
	}

	logging.Infof(ctx, logrus.Fields{
		"component":       "SimulatorClient",
		"command_id":      commandID,
		"external_system": externalSystem,
		"external_id":     externalID,
		"point_key":       command,
		"value":           value,
	}, "simulator: command delivered")
	return nil
}
