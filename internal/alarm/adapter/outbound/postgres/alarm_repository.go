package postgres

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/port"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
)

type AlarmRepository struct {
	db *gorm.DB
}

func NewAlarmRepository(p *platformpostgres.Postgres) *AlarmRepository {
	if p == nil {
		panic("NewAlarmRepository: postgres is required")
	}
	return &AlarmRepository{db: p.DB()}
}

var _ port.AlarmRepository = (*AlarmRepository)(nil)

type ingestScan struct {
	DedupInserted int     `gorm:"column:dedup_inserted"`
	AlarmID       *string `gorm:"column:alarm_id"`
}

func (r *AlarmRepository) Ingest(ctx context.Context, candidateID string, d model.Decision) (port.IngestResult, error) {
	attrs, err := marshalAttributes(d.Attributes)
	if err != nil {
		return port.IngestResult{}, fmt.Errorf("%w: marshal attributes: %v", domain.ErrPoisonStore, err)
	}
	schema := d.AttributesSchema
	if schema == 0 {
		schema = model.AttributesSchemaV1
	}
	args := []any{
		d.TenantID, d.EventID, candidateID,
		candidateID, d.TenantID, d.Fingerprint, d.Severity, d.RuleID,
		d.Title, d.Summary, d.SourceRef, attrs, schema,
		d.OccurredAt, d.OccurredAt, d.EventID,
	}

	sql := soeIngestSQL
	if d.Source == model.SourceDispatch {
		sql = dispatchIngestSQL
	}

	var out ingestScan
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&out).Error; err != nil {
		return port.IngestResult{}, mapDBError(err)
	}
	res := port.IngestResult{DedupInserted: out.DedupInserted}
	if out.AlarmID != nil {
		res.AlarmID = *out.AlarmID
	}
	return res, nil
}

func (r *AlarmRepository) FindByID(ctx context.Context, tenantID, id string) (*model.Alarm, error) {
	var row alarmRow
	err := r.db.WithContext(ctx).Raw(findByIDSQL, tenantID, id).Scan(&row).Error
	if err != nil {
		return nil, mapDBError(err)
	}
	if row.ID == "" {
		return nil, domain.ErrNotFound
	}
	return row.toDomain()
}

func (r *AlarmRepository) List(ctx context.Context, f port.ListFilter) ([]*model.Alarm, int, error) {
	where := "tenant_id = ?"
	args := []any{f.TenantID}
	if f.Status != "" {
		where += " AND status = ?"
		args = append(args, string(f.Status))
	}
	if f.Severity != "" {
		where += " AND severity = ?"
		args = append(args, string(f.Severity))
	}
	if f.Source != "" {
		where += " AND source = ?"
		args = append(args, string(f.Source))
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM alarms WHERE " + where
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, mapDBError(err)
	}

	listSQL := "SELECT" + alarmSelectCols + " FROM alarms WHERE " + where +
		" ORDER BY last_occurred_at DESC LIMIT ? OFFSET ?"
	listArgs := make([]any, 0, len(args)+2)
	listArgs = append(listArgs, args...)
	listArgs = append(listArgs, f.Limit, f.Offset)

	var rows []alarmRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, mapDBError(err)
	}
	items := make([]*model.Alarm, 0, len(rows))
	for _, row := range rows {
		a, convErr := row.toDomain()
		if convErr != nil {
			return nil, 0, convErr
		}
		items = append(items, a)
	}
	return items, int(total), nil
}

func (r *AlarmRepository) Acknowledge(ctx context.Context, tenantID, id string, version int, actor string, at time.Time) (*model.Alarm, error) {
	return r.mutate(ctx, ackSQL, tenantID, id, version, actor, at, func(cur *model.Alarm) error {
		switch cur.Status {
		case model.StatusAcknowledged:
			return domain.ErrAlreadyAcknowledged
		case model.StatusClosed:
			return domain.ErrAlreadyClosed
		default:
			if cur.Version != version {
				return domain.ErrConflict
			}
			return domain.ErrInvalidTransition
		}
	})
}

func (r *AlarmRepository) Close(ctx context.Context, tenantID, id string, version int, actor string, at time.Time) (*model.Alarm, error) {
	return r.mutate(ctx, closeSQL, tenantID, id, version, actor, at, func(cur *model.Alarm) error {
		if cur.Status.IsClosed() {
			return domain.ErrAlreadyClosed
		}
		if cur.Version != version {
			return domain.ErrConflict
		}
		return domain.ErrInvalidTransition
	})
}

func (r *AlarmRepository) mutate(
	ctx context.Context,
	sql string,
	tenantID, id string,
	version int,
	actor string,
	at time.Time,
	onZero func(*model.Alarm) error,
) (*model.Alarm, error) {
	var row alarmRow
	err := r.db.WithContext(ctx).Raw(sql, actor, at, id, tenantID, version).Scan(&row).Error
	if err != nil {
		return nil, mapDBError(err)
	}
	if row.ID != "" {
		return row.toDomain()
	}
	cur, findErr := r.FindByID(ctx, tenantID, id)
	if findErr != nil {
		return nil, findErr
	}
	return nil, onZero(cur)
}

func (r *AlarmRepository) CountOpenBySource(ctx context.Context) (map[model.Source]int, error) {
	var rows []struct {
		Source string `gorm:"column:source"`
		N      int    `gorm:"column:n"`
	}
	if err := r.db.WithContext(ctx).Raw(countOpenBySourceSQL).Scan(&rows).Error; err != nil {
		return nil, mapDBError(err)
	}
	out := map[model.Source]int{
		model.SourceDispatch: 0,
		model.SourceSOE:      0,
	}
	for _, row := range rows {
		out[model.Source(row.Source)] += row.N
	}
	return out, nil
}
