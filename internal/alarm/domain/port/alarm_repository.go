package port

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
)

// IngestResult is the outcome of one atomic dedup+alarm statement.
// DedupInserted==0 means the event_id was already processed: do not bump count.
type IngestResult struct {
	DedupInserted int
	AlarmID       string
}

// OpenedNew reports whether this ingest inserted a brand-new open row
// (candidate id survived). SOE merge returns the existing id.
func (r IngestResult) OpenedNew(candidateID string) bool {
	return r.DedupInserted == 1 && r.AlarmID == candidateID
}

func (r IngestResult) DedupHit() bool { return r.DedupInserted == 0 }

// AlarmRepository is the persistence port for the Alarm aggregate.
//
// Ingest MUST run dedup insert before writing alarms, in a single SQL
// statement. Implementations must not load-then-Touch.
type AlarmRepository interface {
	Ingest(ctx context.Context, candidateID string, d model.Decision) (IngestResult, error)

	FindByID(ctx context.Context, tenantID, id string) (*model.Alarm, error)
	List(ctx context.Context, f ListFilter) (items []*model.Alarm, total int, err error)

	// Acknowledge / Close are optimistic: WHERE version=$v. Zero rows must be
	// classified as not found, already terminal, or ErrConflict — never a silent overwrite.
	Acknowledge(ctx context.Context, tenantID, id string, version int, actor string, at time.Time) (*model.Alarm, error)
	Close(ctx context.Context, tenantID, id string, version int, actor string, at time.Time) (*model.Alarm, error)

	// CountOpenBySource is startup calibration for alarm_open_alarms.
	// Do not call on the Prometheus scrape path.
	CountOpenBySource(ctx context.Context) (map[model.Source]int, error)
}

// ListFilter is tenant-scoped. Zero Status/Severity/Source means "any".
type ListFilter struct {
	TenantID string
	Status   model.Status
	Severity model.Severity
	Source   model.Source
	Offset   int
	Limit    int
}
