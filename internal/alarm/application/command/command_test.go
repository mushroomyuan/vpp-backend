package command

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	dispEvent "github.com/mushroomyuan/vpp-backend/platform/event/dispatch"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/port"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/service"
)

var (
	_ port.AlarmRepository = (*memRepo)(nil)
	_ port.Notifier        = (*recordingNotifier)(nil)
)

type memRepo struct {
	alarms map[string]*model.Alarm // id → alarm
	dedup  map[string]string       // tenant\x00event_id → alarm_id
}

func newMemRepo() *memRepo {
	return &memRepo{alarms: map[string]*model.Alarm{}, dedup: map[string]string{}}
}

func dedupKey(tenantID, eventID string) string { return tenantID + "\x00" + eventID }

func (r *memRepo) Ingest(_ context.Context, candidateID string, d model.Decision) (port.IngestResult, error) {
	if _, ok := r.dedup[dedupKey(d.TenantID, d.EventID)]; ok {
		return port.IngestResult{DedupInserted: 0}, nil
	}
	if d.Source == model.SourceSOE {
		for _, a := range r.alarms {
			if a.TenantID == d.TenantID && a.Fingerprint == d.Fingerprint && !a.Status.IsClosed() {
				a.Count++
				a.LastEventID = d.EventID
				a.Title = d.Title
				a.Summary = d.Summary
				if !d.OccurredAt.Before(a.LastOccurredAt) {
					a.Attributes = d.Attributes
					a.LastOccurredAt = d.OccurredAt
				}
				a.Version++
				r.dedup[dedupKey(d.TenantID, d.EventID)] = a.ID
				return port.IngestResult{DedupInserted: 1, AlarmID: a.ID}, nil
			}
		}
	}
	a, err := model.NewOpenAlarm(candidateID, d)
	if err != nil {
		return port.IngestResult{}, err
	}
	r.alarms[a.ID] = a
	r.dedup[dedupKey(d.TenantID, d.EventID)] = a.ID
	return port.IngestResult{DedupInserted: 1, AlarmID: a.ID}, nil
}

func (r *memRepo) FindByID(_ context.Context, tenantID, id string) (*model.Alarm, error) {
	a, ok := r.alarms[id]
	if !ok || a.TenantID != tenantID {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (r *memRepo) List(_ context.Context, f port.ListFilter) ([]*model.Alarm, int, error) {
	var items []*model.Alarm
	for _, a := range r.alarms {
		if a.TenantID != f.TenantID {
			continue
		}
		if f.Status != "" && a.Status != f.Status {
			continue
		}
		if f.Severity != "" && a.Severity != f.Severity {
			continue
		}
		if f.Source != "" && a.Source != f.Source {
			continue
		}
		cp := *a
		items = append(items, &cp)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].LastOccurredAt.After(items[j].LastOccurredAt)
	})
	total := len(items)
	if f.Offset > len(items) {
		return []*model.Alarm{}, total, nil
	}
	items = items[f.Offset:]
	if f.Limit > 0 && len(items) > f.Limit {
		items = items[:f.Limit]
	}
	return items, total, nil
}

func (r *memRepo) Acknowledge(_ context.Context, tenantID, id string, version int, actor string, at time.Time) (*model.Alarm, error) {
	return r.mutate(tenantID, id, version, func(a *model.Alarm) error {
		return a.Acknowledge(actor, at)
	})
}

func (r *memRepo) Close(_ context.Context, tenantID, id string, version int, actor string, at time.Time) (*model.Alarm, error) {
	return r.mutate(tenantID, id, version, func(a *model.Alarm) error {
		return a.Close(actor, at)
	})
}

func (r *memRepo) CountOpenBySource(_ context.Context) (map[model.Source]int, error) {
	out := map[model.Source]int{model.SourceDispatch: 0, model.SourceSOE: 0}
	for _, a := range r.alarms {
		if !a.Status.IsClosed() {
			out[a.Source]++
		}
	}
	return out, nil
}

func (r *memRepo) mutate(tenantID, id string, version int, fn func(*model.Alarm) error) (*model.Alarm, error) {
	a, ok := r.alarms[id]
	if !ok || a.TenantID != tenantID {
		return nil, domain.ErrNotFound
	}
	if a.Version != version {
		return nil, domain.ErrConflict
	}
	if err := fn(a); err != nil {
		return nil, err
	}
	cp := *a
	return &cp, nil
}

type recordingNotifier struct{ n int }

func (n *recordingNotifier) Notify(context.Context, *model.Alarm) error {
	n.n++
	return nil
}

type recObs struct {
	opened, closed []string
	ackC, closeC   int
	set            map[string]int
}

func (o *recObs) AlarmOpened(source string) { o.opened = append(o.opened, source) }
func (o *recObs) AlarmClosed(source string) { o.closed = append(o.closed, source) }
func (o *recObs) SetOpenCount(source string, n int) {
	if o.set == nil {
		o.set = map[string]int{}
	}
	o.set[source] = n
}
func (o *recObs) AckConflict()   { o.ackC++ }
func (o *recObs) CloseConflict() { o.closeC++ }

var _ port.Observer = (*recObs)(nil)

func newIngestHandler(repo port.AlarmRepository, n port.Notifier) ingestEventHandler {
	return ingestEventHandler{
		evaluator: service.NewEvaluator(service.DefaultRules()),
		repo:      repo,
		notifier:  n,
	}
}

func TestIngestEvent_DispatchOpensTicket(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	note := &recordingNotifier{}
	h := newIngestHandler(repo, note)

	res, err := h.Handle(context.Background(), IngestEvent{Incoming: model.IncomingEvent{
		Source:     model.SourceDispatch,
		TenantID:   "t1",
		EventID:    "evt-1",
		EventType:  dispEvent.TypeTaskFailed,
		OccurredAt: time.Unix(1, 0).UTC(),
		TaskID:     "task-1",
		TaskName:   "shed",
		TaskStatus: "failed",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != OutcomeOK || res.AlarmID == "" || note.n != 1 {
		t.Fatalf("%+v notify=%d", res, note.n)
	}
	got, _ := repo.FindByID(context.Background(), "t1", res.AlarmID)
	if got.Severity != model.SeverityCritical || got.Count != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestIngestEvent_SOEMergeAndDedup(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	note := &recordingNotifier{}
	h := newIngestHandler(repo, note)
	ts1 := time.Unix(10, 0).UTC()
	ts2 := time.Unix(20, 0).UTC()
	in := func(ts time.Time, old, new float64) IngestEvent {
		return IngestEvent{Incoming: model.IncomingEvent{
			Source: model.SourceSOE, TenantID: "t1", OccurredAt: ts,
			CUCode: "cu", MetricName: "brk", OldValue: old, NewValue: new,
		}}
	}

	r1, err := h.Handle(context.Background(), in(ts1, 0, 1))
	if err != nil || r1.Outcome != OutcomeOK {
		t.Fatalf("%+v %v", r1, err)
	}
	r2, err := h.Handle(context.Background(), in(ts2, 1, 0))
	if err != nil || r2.Outcome != OutcomeOK || r2.AlarmID != r1.AlarmID {
		t.Fatalf("merge %+v %v", r2, err)
	}
	if note.n != 1 {
		t.Fatalf("notify on merge: %d", note.n)
	}
	got, _ := repo.FindByID(context.Background(), "t1", r1.AlarmID)
	if got.Count != 2 || got.LastOccurredAt.Equal(ts1) {
		t.Fatalf("count/last %+v", got)
	}

	r3, err := h.Handle(context.Background(), in(ts1, 0, 1)) // replay first event_id
	if err != nil || r3.Outcome != OutcomeDedupHit {
		t.Fatalf("dedup %+v %v", r3, err)
	}
	got, _ = repo.FindByID(context.Background(), "t1", r1.AlarmID)
	if got.Count != 2 {
		t.Fatalf("dedup bumped count to %d", got.Count)
	}
}

func TestIngestEvent_CloseThenNewSOE(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	h := newIngestHandler(repo, &recordingNotifier{})
	in := IngestEvent{Incoming: model.IncomingEvent{
		Source: model.SourceSOE, TenantID: "t1", OccurredAt: time.Unix(1, 0).UTC(),
		CUCode: "cu", MetricName: "brk", OldValue: 0, NewValue: 1,
	}}
	r1, _ := h.Handle(context.Background(), in)
	ackH := closeHandler{repo: repo}
	if _, err := ackH.Handle(context.Background(), Close{
		TenantID: "t1", AlarmID: r1.AlarmID, Version: 1, Actor: "op",
	}); err != nil {
		t.Fatal(err)
	}
	in.Incoming.OccurredAt = time.Unix(2, 0).UTC()
	r2, err := h.Handle(context.Background(), in)
	if err != nil || r2.Outcome != OutcomeOK || r2.AlarmID == r1.AlarmID {
		t.Fatalf("want new ticket %+v %v", r2, err)
	}
}

func TestIngestEvent_Dropped(t *testing.T) {
	t.Parallel()
	h := newIngestHandler(newMemRepo(), &recordingNotifier{})
	res, err := h.Handle(context.Background(), IngestEvent{Incoming: model.IncomingEvent{
		Source:     model.SourceDispatch,
		TenantID:   "t1",
		EventID:    "evt",
		EventType:  dispEvent.TypeTaskCompleted,
		OccurredAt: time.Unix(1, 0).UTC(),
		TaskID:     "task",
	}})
	if err != nil || res.Outcome != OutcomeDropped {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestAcknowledge_VersionConflictDoesNotOverwrite(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	h := newIngestHandler(repo, &recordingNotifier{})
	res, _ := h.Handle(context.Background(), IngestEvent{Incoming: model.IncomingEvent{
		Source:     model.SourceDispatch,
		TenantID:   "t1",
		EventID:    "evt-1",
		EventType:  dispEvent.TypeTaskFailed,
		OccurredAt: time.Unix(1, 0).UTC(),
		TaskID:     "task-1",
		TaskName:   "shed",
		TaskStatus: "failed",
	}})
	ack := acknowledgeHandler{repo: repo}
	if _, err := ack.Handle(context.Background(), Acknowledge{
		TenantID: "t1", AlarmID: res.AlarmID, Version: 1, Actor: "alice",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := ack.Handle(context.Background(), Acknowledge{
		TenantID: "t1", AlarmID: res.AlarmID, Version: 1, Actor: "bob",
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("got %v", err)
	}
	got, _ := repo.FindByID(context.Background(), "t1", res.AlarmID)
	if got.AcknowledgedBy != "alice" {
		t.Fatalf("overwritten by %s", got.AcknowledgedBy)
	}
}

func TestIngest_ObserverOpenedOnNewNotOnMerge(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	obs := &recObs{}
	h := ingestEventHandler{
		evaluator: service.NewEvaluator(service.DefaultRules()),
		repo:      repo,
		notifier:  &recordingNotifier{},
		observer:  obs,
	}
	in := func(ts time.Time, old, new float64) IngestEvent {
		return IngestEvent{Incoming: model.IncomingEvent{
			Source: model.SourceSOE, TenantID: "t1", OccurredAt: ts,
			CUCode: "cu", MetricName: "brk", OldValue: old, NewValue: new,
		}}
	}
	r1, err := h.Handle(context.Background(), in(time.Unix(1, 0).UTC(), 0, 1))
	if err != nil || !r1.Opened {
		t.Fatalf("%+v %v", r1, err)
	}
	r2, err := h.Handle(context.Background(), in(time.Unix(2, 0).UTC(), 1, 0))
	if err != nil || r2.Opened {
		t.Fatalf("merge must not open %+v %v", r2, err)
	}
	if len(obs.opened) != 1 || obs.opened[0] != string(model.SourceSOE) {
		t.Fatalf("opened=%v", obs.opened)
	}

	closeH := closeHandler{repo: repo, observer: obs}
	if _, err := closeH.Handle(context.Background(), Close{
		TenantID: "t1", AlarmID: r1.AlarmID, Version: 2, Actor: "op",
	}); err != nil {
		t.Fatal(err)
	}
	if len(obs.closed) != 1 || obs.closed[0] != string(model.SourceSOE) {
		t.Fatalf("closed=%v", obs.closed)
	}
}

func TestAcknowledge_ObserverConflict(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	obs := &recObs{}
	h := newIngestHandler(repo, &recordingNotifier{})
	res, _ := h.Handle(context.Background(), IngestEvent{Incoming: model.IncomingEvent{
		Source: model.SourceDispatch, TenantID: "t1", EventID: "evt-1",
		EventType: dispEvent.TypeTaskFailed, OccurredAt: time.Unix(1, 0).UTC(),
		TaskID: "task-1", TaskName: "shed", TaskStatus: "failed",
	}})
	ack := acknowledgeHandler{repo: repo, observer: obs}
	if _, err := ack.Handle(context.Background(), Acknowledge{
		TenantID: "t1", AlarmID: res.AlarmID, Version: 1, Actor: "alice",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := ack.Handle(context.Background(), Acknowledge{
		TenantID: "t1", AlarmID: res.AlarmID, Version: 1, Actor: "bob",
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("got %v", err)
	}
	if obs.ackC != 1 {
		t.Fatalf("ack conflict metric=%d", obs.ackC)
	}
}

func TestClose_ObserverConflict(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	obs := &recObs{}
	h := newIngestHandler(repo, &recordingNotifier{})
	res, _ := h.Handle(context.Background(), IngestEvent{Incoming: model.IncomingEvent{
		Source: model.SourceDispatch, TenantID: "t1", EventID: "evt-1",
		EventType: dispEvent.TypeTaskFailed, OccurredAt: time.Unix(1, 0).UTC(),
		TaskID: "task-1", TaskName: "shed", TaskStatus: "failed",
	}})
	clo := closeHandler{repo: repo, observer: obs}
	if _, err := clo.Handle(context.Background(), Close{
		TenantID: "t1", AlarmID: res.AlarmID, Version: 99, Actor: "op",
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("got %v", err)
	}
	if obs.closeC != 1 {
		t.Fatalf("close conflict metric=%d", obs.closeC)
	}
	if len(obs.closed) != 0 {
		t.Fatal("conflict must not decrement open gauge")
	}
}
