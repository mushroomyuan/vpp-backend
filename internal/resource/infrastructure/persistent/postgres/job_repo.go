package postgres

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

const (
	// stuckJobTimeout is the duration after which a running job is considered
	// stuck (worker crashed or was killed) and eligible for reclaim.
	stuckJobTimeout = 10 * time.Minute
)

type JobRepository struct {
	pg *Postgres
}

func NewJobRepository(pg *Postgres) *JobRepository {
	return &JobRepository{pg: pg}
}

func (r *JobRepository) CreateJob(ctx context.Context, m *JobModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "JobRepository.CreateJob", m)
	defer func() { deferLog(nil, &err) }()
	return r.pg.DB().WithContext(ctx).Create(m).Error
}

func (r *JobRepository) FindJobByID(ctx context.Context, id string) (result *JobModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "JobRepository.FindJobByID", id)
	defer func() { deferLog(result, &err) }()
	var m JobModel
	err = r.pg.DB().WithContext(ctx).Where("id = ?", id).First(&m).Error
	if err != nil {
		return nil, err
	}
	result = &m
	return
}

// ClaimPendingJob atomically claims one pending (or stuck-running) job
// and transitions it to running in a single SQL round-trip:
//
//	UPDATE import_jobs
//	SET status='running', attempts=attempts+1, started_at=now
//	WHERE id = (
//	    SELECT id ... FOR UPDATE SKIP LOCKED
//	)
//	RETURNING *
//
// This eliminates the two-step SELECT + UPDATE and the wrapping transaction.
// Two conditions are eligible:
//  1. Pending jobs whose next_retry_at has passed (or has never been set).
//  2. Running jobs whose started_at is older than stuckJobTimeout — these are
//     assumed to belong to a crashed worker and are safe to reclaim.
func (r *JobRepository) ClaimPendingJob(ctx context.Context) (result *JobModel, err error) {
	_, deferLog := logging.WhenDB(ctx, "JobRepository.ClaimPendingJob", nil)
	defer func() {
		if err == nil && result == nil {
			return
		}
		deferLog(result, &err)
	}()

	now := time.Now()
	stuckThreshold := now.Add(-stuckJobTimeout)

	var m JobModel
	tx := r.pg.DB().WithContext(ctx).Raw(`
		UPDATE import_jobs
		SET    status     = 'running',
		       attempts   = attempts + 1,
		       started_at = ?,
		       updated_at = ?
		WHERE  id = (
		    SELECT id
		    FROM   import_jobs
		    WHERE  attempts < max_attempts
		      AND  (
		               (status = 'pending'
		                AND (next_retry_at IS NULL OR next_retry_at <= ?))
		            OR (status = 'running'
		                AND started_at < ?)
		           )
		    ORDER BY created_at ASC
		    LIMIT  1
		    FOR UPDATE SKIP LOCKED
		)
		RETURNING *
	`, now, now, now, stuckThreshold).Scan(&m)

	if tx.Error != nil {
		return nil, tx.Error
	}
	if m.ID == "" {
		return nil, nil // no eligible jobs in the queue
	}
	return &m, nil
}

// SaveJob persists the full current row for an existing import_jobs row. Callers
// are expected to mutate a domain Job and map it to JobModel first.
func (r *JobRepository) SaveJob(ctx context.Context, m *JobModel) (err error) {
	_, deferLog := logging.WhenDB(ctx, "JobRepository.SaveJob", m.ID)
	defer func() { deferLog(nil, &err) }()
	now := time.Now()
	return r.pg.DB().WithContext(ctx).
		Model(&JobModel{}).
		Where("id = ?", m.ID).
		Updates(map[string]any{
			"tenant_id":      m.TenantID,
			"operation_type": m.OperationType,
			"target_type":    m.TargetType,
			"status":         m.Status,
			"payload":        m.Payload,
			"total":          m.Total,
			"succeeded":      m.Succeeded,
			"failed_count":   m.FailedCount,
			"error_msg":      m.ErrorMsg,
			"result_json":    m.ResultJSON,
			"attempts":       m.Attempts,
			"max_attempts":   m.MaxAttempts,
			"started_at":     m.StartedAt,
			"finished_at":    m.FinishedAt,
			"next_retry_at":  m.NextRetryAt,
			"updated_at":     now,
		}).Error
}
