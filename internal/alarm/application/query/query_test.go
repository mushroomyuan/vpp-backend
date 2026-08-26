package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/port"
)

type stubRepo struct {
	items []*model.Alarm
}

func (r *stubRepo) Ingest(context.Context, string, model.Decision) (port.IngestResult, error) {
	return port.IngestResult{}, errors.New("unused")
}
func (r *stubRepo) Acknowledge(context.Context, string, string, int, string, time.Time) (*model.Alarm, error) {
	return nil, errors.New("unused")
}
func (r *stubRepo) Close(context.Context, string, string, int, string, time.Time) (*model.Alarm, error) {
	return nil, errors.New("unused")
}
func (r *stubRepo) CountOpenBySource(context.Context) (map[model.Source]int, error) {
	return map[model.Source]int{}, nil
}
func (r *stubRepo) FindByID(_ context.Context, tenantID, id string) (*model.Alarm, error) {
	for _, a := range r.items {
		if a.ID == id && a.TenantID == tenantID {
			return a, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *stubRepo) List(_ context.Context, f port.ListFilter) ([]*model.Alarm, int, error) {
	var out []*model.Alarm
	for _, a := range r.items {
		if a.TenantID != f.TenantID {
			continue
		}
		if f.Status != "" && a.Status != f.Status {
			continue
		}
		out = append(out, a)
	}
	return out, len(out), nil
}

func sample(id, tenant string, status model.Status) *model.Alarm {
	a, err := model.NewOpenAlarm(id, model.Decision{
		TenantID: tenant, Source: model.SourceSOE, Severity: model.SeverityWarning,
		RuleID: model.RuleSOEDiscreteChange, Title: "x", Fingerprint: "v1:fp-" + id,
		EventID: "e-" + id, SourceRef: "cu/m", AttributesSchema: model.AttributesSchemaV1,
		OccurredAt: time.Unix(1, 0).UTC(),
	})
	if err != nil {
		panic(err)
	}
	a.Status = status
	return a
}

func TestListAlarms_TenantAndStatus(t *testing.T) {
	t.Parallel()
	repo := &stubRepo{items: []*model.Alarm{
		sample("a1", "t1", model.StatusOpen),
		sample("a2", "t1", model.StatusClosed),
		sample("a3", "t2", model.StatusOpen),
	}}
	h := listAlarmsHandler{repo: repo}
	res, err := h.Handle(context.Background(), ListAlarms{TenantID: "t1", Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || res.Alarms[0].ID != "a1" {
		t.Fatalf("%+v", res)
	}
}

func TestListAlarms_InvalidStatus(t *testing.T) {
	t.Parallel()
	h := listAlarmsHandler{repo: &stubRepo{}}
	_, err := h.Handle(context.Background(), ListAlarms{TenantID: "t1", Status: "nope"})
	if !errors.Is(err, domain.ErrInvalidFilter) {
		t.Fatalf("got %v", err)
	}
}

func TestGetAlarm_TenantScoped(t *testing.T) {
	t.Parallel()
	repo := &stubRepo{items: []*model.Alarm{sample("a1", "t1", model.StatusOpen)}}
	h := getAlarmHandler{repo: repo}
	if _, err := h.Handle(context.Background(), GetAlarm{TenantID: "t2", AlarmID: "a1"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	res, err := h.Handle(context.Background(), GetAlarm{TenantID: "t1", AlarmID: "a1"})
	if err != nil || res.Alarm.ID != "a1" {
		t.Fatalf("%+v %v", res, err)
	}
}
