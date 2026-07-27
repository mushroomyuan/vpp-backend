package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/mushroomyuan/vpp-backend/gateway/application/command"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event"
	resEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
)

type stubDisableHandler struct {
	calls []command.DisableMappingByCUCode
	err   error
}

func (h *stubDisableHandler) Handle(_ context.Context, cmd command.DisableMappingByCUCode) (struct{}, error) {
	h.calls = append(h.calls, cmd)
	return struct{}{}, h.err
}

func TestIsInactiveStatus(t *testing.T) {
	t.Parallel()
	if !isInactiveStatus("disabled") || !isInactiveStatus("archived") {
		t.Fatal("want inactive true")
	}
	if isInactiveStatus("active") || isInactiveStatus("enabled") || isInactiveStatus("") {
		t.Fatal("want inactive false")
	}
}

func TestPeekEventType(t *testing.T) {
	t.Parallel()
	if got := peekEventType([]byte(`{"event_type":"resource.deleted"}`)); got != "resource.deleted" {
		t.Fatalf("got %q", got)
	}
	if got := peekEventType([]byte(`not-json`)); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestHandleMessage_ResourceDeleted(t *testing.T) {
	t.Parallel()
	h := &stubDisableHandler{}
	c := &LifecycleConsumer{handler: h}

	env := platEvent.Envelope[resEvent.ResourceDeletedPayload]{
		EventID: "e1", EventType: resEvent.TypeResourceDeleted, TenantID: "tenant-env",
		OccurredAt: time.Now(),
		Payload:    resEvent.ResourceDeletedPayload{ResourceID: "cu-99", TenantID: "tenant-payload"},
	}
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.handleMessage(context.Background(), kafka.Message{Value: body}); err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 1 || h.calls[0].TenantID != "tenant-env" || h.calls[0].CUCode != "cu-99" {
		t.Fatalf("calls = %+v", h.calls)
	}
}

func TestHandleMessage_ResourceDeleted_TenantFallback(t *testing.T) {
	t.Parallel()
	h := &stubDisableHandler{}
	c := &LifecycleConsumer{handler: h}

	env := platEvent.Envelope[resEvent.ResourceDeletedPayload]{
		EventType: resEvent.TypeResourceDeleted,
		Payload:   resEvent.ResourceDeletedPayload{ResourceID: "cu-1", TenantID: "from-payload"},
	}
	body, _ := json.Marshal(env)
	if err := c.handleMessage(context.Background(), kafka.Message{Value: body}); err != nil {
		t.Fatal(err)
	}
	if h.calls[0].TenantID != "from-payload" {
		t.Fatalf("tenant = %q", h.calls[0].TenantID)
	}
}

func TestHandleMessage_LifecycleChanged(t *testing.T) {
	t.Parallel()

	t.Run("active skipped", func(t *testing.T) {
		t.Parallel()
		h := &stubDisableHandler{}
		c := &LifecycleConsumer{handler: h}
		env := platEvent.Envelope[resEvent.LifecycleChangedPayload]{
			EventType: resEvent.TypeLifecycleChanged, TenantID: "t",
			Payload: resEvent.LifecycleChangedPayload{ResourceID: "cu", Status: "active"},
		}
		body, _ := json.Marshal(env)
		if err := c.handleMessage(context.Background(), kafka.Message{Value: body}); err != nil {
			t.Fatal(err)
		}
		if len(h.calls) != 0 {
			t.Fatal("should skip active")
		}
	})

	t.Run("disabled triggers", func(t *testing.T) {
		t.Parallel()
		h := &stubDisableHandler{}
		c := &LifecycleConsumer{handler: h}
		env := platEvent.Envelope[resEvent.LifecycleChangedPayload]{
			EventType: resEvent.TypeLifecycleChanged, TenantID: "t",
			Payload: resEvent.LifecycleChangedPayload{ResourceID: "cu-2", Status: "disabled"},
		}
		body, _ := json.Marshal(env)
		if err := c.handleMessage(context.Background(), kafka.Message{Value: body}); err != nil {
			t.Fatal(err)
		}
		if len(h.calls) != 1 || h.calls[0].CUCode != "cu-2" {
			t.Fatalf("calls = %+v", h.calls)
		}
	})
}

func TestHandleMessage_SkipUnknownAndBadJSON(t *testing.T) {
	t.Parallel()
	h := &stubDisableHandler{}
	c := &LifecycleConsumer{handler: h}

	if err := c.handleMessage(context.Background(), kafka.Message{Value: []byte(`not-json`)}); err != nil {
		t.Fatal(err)
	}
	env := platEvent.Envelope[map[string]any]{
		EventType: "resource.created", TenantID: "t", Payload: map[string]any{},
	}
	body, _ := json.Marshal(env)
	if err := c.handleMessage(context.Background(), kafka.Message{Value: body}); err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 0 {
		t.Fatal("unknown/bad should not call handler")
	}
}

func TestHandleMessage_HandlerError(t *testing.T) {
	t.Parallel()
	h := &stubDisableHandler{err: errors.New("disable failed")}
	c := &LifecycleConsumer{handler: h}
	env := platEvent.Envelope[resEvent.ResourceDeletedPayload]{
		EventType: resEvent.TypeResourceDeleted, TenantID: "t",
		Payload: resEvent.ResourceDeletedPayload{ResourceID: "cu"},
	}
	body, _ := json.Marshal(env)
	if err := c.handleMessage(context.Background(), kafka.Message{Value: body}); err == nil {
		t.Fatal("want error")
	}
}
