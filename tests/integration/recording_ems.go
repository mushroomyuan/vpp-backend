package integration

import (
	"context"
	"sync"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
)

// recordedCommand is one setpoint Gateway handed to the device adapter.
type recordedCommand struct {
	CommandID      string
	ExternalSystem string
	ExternalID     string
	Address        string
	Value          float64
}

// recordingEMS keeps every command Gateway sends and forwards it to next.
type recordingEMS struct {
	next port.EMSClient

	mu   sync.Mutex
	sent []recordedCommand
}

func (r *recordingEMS) SendCommand(
	ctx context.Context,
	commandID, externalSystem, externalID, command string,
	value float64,
) error {
	r.mu.Lock()
	r.sent = append(r.sent, recordedCommand{
		CommandID:      commandID,
		ExternalSystem: externalSystem,
		ExternalID:     externalID,
		Address:        command,
		Value:          value,
	})
	r.mu.Unlock()
	if r.next == nil {
		return nil
	}
	return r.next.SendCommand(ctx, commandID, externalSystem, externalID, command, value)
}

func (r *recordingEMS) byID(commandID string) (recordedCommand, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.sent) - 1; i >= 0; i-- {
		if r.sent[i].CommandID == commandID {
			return r.sent[i], true
		}
	}
	return recordedCommand{}, false
}
