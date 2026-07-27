package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/gateway/domain"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
)

type stubMappingRepo struct {
	byExternal *model.DeviceMapping
	byCU       *model.DeviceMapping
	getExtErr  error
	getCUErr   error
	disabledID string
	disableErr error
}

func (r *stubMappingRepo) Create(context.Context, *model.DeviceMapping) error {
	return errors.New("not implemented")
}
func (r *stubMappingRepo) Delete(context.Context, string, string) error {
	return errors.New("not implemented")
}
func (r *stubMappingRepo) List(context.Context, string) ([]*model.DeviceMapping, error) {
	return nil, errors.New("not implemented")
}

func (r *stubMappingRepo) Disable(_ context.Context, _, id string) error {
	if r.disableErr != nil {
		return r.disableErr
	}
	r.disabledID = id
	return nil
}

func (r *stubMappingRepo) GetByExternalID(context.Context, string, string, string) (*model.DeviceMapping, error) {
	if r.getExtErr != nil {
		return nil, r.getExtErr
	}
	return r.byExternal, nil
}

func (r *stubMappingRepo) GetByCUCode(context.Context, string, string) (*model.DeviceMapping, error) {
	if r.getCUErr != nil {
		return nil, r.getCUErr
	}
	return r.byCU, nil
}

type stubTelemetryClient struct {
	last *model.StandardTelemetry
	err  error
	n    int
}

func (c *stubTelemetryClient) Ingest(_ context.Context, t *model.StandardTelemetry) error {
	c.n++
	c.last = t
	return c.err
}

type stubEMSClient struct {
	n            int
	lastSystem   string
	lastExtID    string
	lastCommand  string
	lastValue    float64
	err          error
}

func (c *stubEMSClient) SendCommand(_ context.Context, _, externalSystem, externalID, command string, value float64) error {
	c.n++
	c.lastSystem = externalSystem
	c.lastExtID = externalID
	c.lastCommand = command
	c.lastValue = value
	return c.err
}

type stubPublisher struct {
	n     int
	last  port.CommandCompletedEvent
	err   error
}

func (p *stubPublisher) PublishCommandCompleted(_ context.Context, event port.CommandCompletedEvent) error {
	p.n++
	p.last = event
	return p.err
}

func activeMapping() *model.DeviceMapping {
	return &model.DeviceMapping{
		ID:             "m1",
		TenantID:       "tenant",
		ExternalSystem: "ems-sg",
		ExternalID:     "dev-1",
		CUCode:         "cu-1",
		Status:         model.MappingStatusActive,
	}
}

func TestReceiveTelemetry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("success translates to standard", func(t *testing.T) {
		t.Parallel()
		repo := &stubMappingRepo{byExternal: activeMapping()}
		tel := &stubTelemetryClient{}
		h := receiveTelemetryHandler{mappingRepo: repo, telemetryClient: tel}

		_, err := h.Handle(ctx, ReceiveTelemetry{Telemetry: &model.ExternalTelemetry{
			TenantID: "tenant", ExternalSystem: "ems-sg", ExternalID: "dev-1",
			Timestamp: time.Unix(1700000000, 0).UTC(),
			Metrics:   []model.ExternalMetric{{Name: "power", Value: 12.5}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if tel.n != 1 || tel.last == nil {
			t.Fatal("Ingest not called")
		}
		if tel.last.CUCode != "cu-1" || tel.last.TenantID != "tenant" {
			t.Fatalf("mapped fields: %+v", tel.last)
		}
		if len(tel.last.Metrics) != 1 ||
			tel.last.Metrics[0].Type != model.MetricTypeAnalog ||
			tel.last.Metrics[0].Quality != model.QualityGood ||
			tel.last.Metrics[0].Value != 12.5 {
			t.Fatalf("metrics = %+v", tel.last.Metrics)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		t.Parallel()
		h := receiveTelemetryHandler{mappingRepo: &stubMappingRepo{}, telemetryClient: &stubTelemetryClient{}}
		_, err := h.Handle(ctx, ReceiveTelemetry{Telemetry: &model.ExternalTelemetry{}})
		if err == nil {
			t.Fatal("want validation error")
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		repo := &stubMappingRepo{getExtErr: domain.ErrMappingNotFound}
		tel := &stubTelemetryClient{}
		h := receiveTelemetryHandler{mappingRepo: repo, telemetryClient: tel}
		_, err := h.Handle(ctx, ReceiveTelemetry{Telemetry: &model.ExternalTelemetry{
			TenantID: "t", ExternalSystem: "s", ExternalID: "e",
			Timestamp: time.Now(), Metrics: []model.ExternalMetric{{Name: "p", Value: 1}},
		}})
		if !errors.Is(err, domain.ErrMappingNotFound) || tel.n != 0 {
			t.Fatalf("err=%v ingest=%d", err, tel.n)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		m := activeMapping()
		m.Status = model.MappingStatusDisabled
		repo := &stubMappingRepo{byExternal: m}
		tel := &stubTelemetryClient{}
		h := receiveTelemetryHandler{mappingRepo: repo, telemetryClient: tel}
		_, err := h.Handle(ctx, ReceiveTelemetry{Telemetry: &model.ExternalTelemetry{
			TenantID: "tenant", ExternalSystem: "ems-sg", ExternalID: "dev-1",
			Timestamp: time.Now(), Metrics: []model.ExternalMetric{{Name: "p", Value: 1}},
		}})
		if !errors.Is(err, domain.ErrMappingDisabled) || tel.n != 0 {
			t.Fatalf("err=%v ingest=%d", err, tel.n)
		}
	})

	t.Run("ingest failure", func(t *testing.T) {
		t.Parallel()
		repo := &stubMappingRepo{byExternal: activeMapping()}
		tel := &stubTelemetryClient{err: errors.New("downstream")}
		h := receiveTelemetryHandler{mappingRepo: repo, telemetryClient: tel}
		_, err := h.Handle(ctx, ReceiveTelemetry{Telemetry: &model.ExternalTelemetry{
			TenantID: "tenant", ExternalSystem: "ems-sg", ExternalID: "dev-1",
			Timestamp: time.Now(), Metrics: []model.ExternalMetric{{Name: "p", Value: 1}},
		}})
		if err == nil {
			t.Fatal("want error")
		}
	})
}

func TestExecuteCommand(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	valid := ExecuteCommand{
		CommandID: "cmd-1", TenantID: "tenant", CUCode: "cu-1", PointKey: "set_power", Value: 10,
	}

	t.Run("validation", func(t *testing.T) {
		t.Parallel()
		h := executeCommandHandler{
			mappingRepo: &stubMappingRepo{}, emsClient: &stubEMSClient{}, publisher: &stubPublisher{},
		}
		for _, cmd := range []ExecuteCommand{
			{TenantID: "", CUCode: "c", PointKey: "p", CommandID: "i"},
			{TenantID: "t", CUCode: "", PointKey: "p", CommandID: "i"},
			{TenantID: "t", CUCode: "c", PointKey: "", CommandID: "i"},
			{TenantID: "t", CUCode: "c", PointKey: "p", CommandID: ""},
		} {
			if _, err := h.Handle(ctx, cmd); err == nil {
				t.Fatalf("want error for %+v", cmd)
			}
		}
	})

	t.Run("success publishes completed", func(t *testing.T) {
		t.Parallel()
		repo := &stubMappingRepo{byCU: activeMapping()}
		ems := &stubEMSClient{}
		pub := &stubPublisher{}
		h := executeCommandHandler{mappingRepo: repo, emsClient: ems, publisher: pub}

		res, err := h.Handle(ctx, valid)
		if err != nil {
			t.Fatal(err)
		}
		if res.ExternalSystem != "ems-sg" || res.ExternalID != "dev-1" {
			t.Fatalf("res = %+v", res)
		}
		if ems.n != 1 || ems.lastCommand != "set_power" || ems.lastValue != 10 {
			t.Fatalf("ems = %+v", ems)
		}
		if pub.n != 1 || !pub.last.Success || pub.last.CommandID != "cmd-1" {
			t.Fatalf("pub = %+v", pub.last)
		}
	})

	t.Run("publish failure still succeeds", func(t *testing.T) {
		t.Parallel()
		repo := &stubMappingRepo{byCU: activeMapping()}
		h := executeCommandHandler{
			mappingRepo: repo,
			emsClient:   &stubEMSClient{},
			publisher:   &stubPublisher{err: errors.New("kafka down")},
		}
		res, err := h.Handle(ctx, valid)
		if err != nil || res == nil {
			t.Fatalf("err=%v res=%v", err, res)
		}
	})

	t.Run("not found and disabled", func(t *testing.T) {
		t.Parallel()
		ems := &stubEMSClient{}
		h1 := executeCommandHandler{
			mappingRepo: &stubMappingRepo{getCUErr: domain.ErrMappingNotFound},
			emsClient:   ems, publisher: &stubPublisher{},
		}
		if _, err := h1.Handle(ctx, valid); !errors.Is(err, domain.ErrMappingNotFound) || ems.n != 0 {
			t.Fatalf("not found: err=%v n=%d", err, ems.n)
		}

		m := activeMapping()
		m.Disable()
		h2 := executeCommandHandler{
			mappingRepo: &stubMappingRepo{byCU: m},
			emsClient:   ems, publisher: &stubPublisher{},
		}
		if _, err := h2.Handle(ctx, valid); !errors.Is(err, domain.ErrMappingDisabled) {
			t.Fatalf("disabled: %v", err)
		}
	})
}

func TestDisableMappingByCUCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := disableMappingByCUCodeHandler{repo: &stubMappingRepo{}}

	if _, err := h.Handle(ctx, DisableMappingByCUCode{}); err == nil {
		t.Fatal("want validation error")
	}

	repoMiss := &stubMappingRepo{getCUErr: domain.ErrMappingNotFound}
	hMiss := disableMappingByCUCodeHandler{repo: repoMiss}
	if _, err := hMiss.Handle(ctx, DisableMappingByCUCode{TenantID: "t", CUCode: "cu"}); err != nil {
		t.Fatalf("idempotent miss: %v", err)
	}
	if repoMiss.disabledID != "" {
		t.Fatal("should not Disable on miss")
	}

	repo := &stubMappingRepo{byCU: activeMapping()}
	hOK := disableMappingByCUCodeHandler{repo: repo}
	if _, err := hOK.Handle(ctx, DisableMappingByCUCode{TenantID: "tenant", CUCode: "cu-1"}); err != nil {
		t.Fatal(err)
	}
	if repo.disabledID != "m1" {
		t.Fatalf("disabledID = %q", repo.disabledID)
	}
}
