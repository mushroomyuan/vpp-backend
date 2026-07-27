package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/mushroomyuan/vpp-backend/dispatch/application/command"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event"
	gwEvent "github.com/mushroomyuan/vpp-backend/platform/event/gateway"
)

type stubResultHandler struct {
	calls []command.HandleCommandResult
	err   error
}

func (h *stubResultHandler) Handle(_ context.Context, cmd command.HandleCommandResult) (any, error) {
	h.calls = append(h.calls, cmd)
	return nil, h.err
}

func TestPeekEventType(t *testing.T) {
	t.Parallel()
	if got := peekEventType([]byte(`{"event_type":"command.completed"}`)); got != "command.completed" {
		t.Fatal(got)
	}
}

func TestHandleMessage_CommandCompleted(t *testing.T) {
	t.Parallel()
	h := &stubResultHandler{}
	c := &CommandResultConsumer{handler: h}
	ack := time.Unix(100, 0).UTC()
	env := platEvent.Envelope[gwEvent.CommandCompletedPayload]{
		EventType: gwEvent.TypeCommandCompleted, TenantID: "ten",
		OccurredAt: time.Now(),
		Payload: gwEvent.CommandCompletedPayload{
			CommandID: "cmd-1", Success: true, AckAt: &ack,
		},
	}
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.handleMessage(context.Background(), kafka.Message{Value: body}); err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 1 || h.calls[0].CommandID != "cmd-1" || !h.calls[0].Result.Success {
		t.Fatalf("%+v", h.calls)
	}
}

func TestHandleMessage_UnknownAndBadJSON(t *testing.T) {
	t.Parallel()
	h := &stubResultHandler{}
	c := &CommandResultConsumer{handler: h}

	if err := c.handleMessage(context.Background(), kafka.Message{Value: []byte(`not-json`)}); err == nil {
		t.Fatal("want unmarshal error")
	}

	env := platEvent.Envelope[map[string]any]{
		EventType: "other.event", Payload: map[string]any{},
	}
	body, _ := json.Marshal(env)
	if err := c.handleMessage(context.Background(), kafka.Message{Value: body}); err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 0 {
		t.Fatal("unknown should not call handler")
	}
}

func TestHandleMessage_HandlerError(t *testing.T) {
	t.Parallel()
	h := &stubResultHandler{err: errors.New("boom")}
	c := &CommandResultConsumer{handler: h}
	env := platEvent.Envelope[gwEvent.CommandCompletedPayload]{
		EventType: gwEvent.TypeCommandCompleted,
		Payload:   gwEvent.CommandCompletedPayload{CommandID: "c", Success: false},
	}
	body, _ := json.Marshal(env)
	if err := c.handleMessage(context.Background(), kafka.Message{Value: body}); err == nil {
		t.Fatal("want error")
	}
}
