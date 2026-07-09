package emslog

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
)

// EMSLogClient is the v1 implementation of port.EMSClient.
// It logs the command and returns nil, enabling the full dispatch path to be
// exercised end-to-end before a real EMS adapter is available.
//
// To integrate a real EMS, add a new adapter package (e.g. adapter/outbound/ems_xxx/)
// that implements port.EMSClient and swap it in server.go — no application layer
// changes required.
type EMSLogClient struct{}

var _ port.EMSClient = (*EMSLogClient)(nil)

func NewEMSLogClient() *EMSLogClient {
	return &EMSLogClient{}
}

func (c *EMSLogClient) SendCommand(
	ctx context.Context,
	commandID, externalSystem, externalID, command string,
	value float64,
) error {
	logrus.WithFields(logrus.Fields{
		"command_id":      commandID,
		"external_system": externalSystem,
		"external_id":     externalID,
		"command":         command,
		"value":           value,
	}).Info("ems_log: command dispatched (log-only, no real EMS connection)")
	return nil
}
