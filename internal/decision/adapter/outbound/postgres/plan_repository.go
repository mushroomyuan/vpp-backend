package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
)

const (
	outboxPending = "pending"
	outboxClaimed = "claimed"
	outboxDone    = "done"
	outboxStale   = "stale"
	outboxFailed  = "failed"
)

var errClaimContended = errors.New("plan: claim contended")

// PlanRepository writes the objective, plan, and outbox in one transaction
// and leases outbox rows for PlanExecutionLoop.
type PlanRepository struct {
	db    *gorm.DB
	newID func() string
}

// NewPlanRepository binds the repository to the decision database.
func NewPlanRepository(pg *platformpostgres.Postgres) *PlanRepository {
	if pg == nil {
		panic("NewPlanRepository: postgres is required")
	}
	return &PlanRepository{db: pg.DB(), newID: idgen.Must}
}

var _ plan.Repository = (*PlanRepository)(nil)

func (r *PlanRepository) Save(ctx context.Context, in plan.SaveInput) (err error) {
	planID := ""
	if in.Plan != nil {
		planID = in.Plan.ID
	}
	_, deferLog := logging.WhenDB(ctx, "PlanRepository.Save", planID)
	defer func() { deferLog(planID, &err) }()
	if err = validateSave(in); err != nil {
		return err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		objRow, powerRow := objectiveModels(in.Objective, in.Now)
		if err := tx.Create(&objRow).Error; err != nil {
			return err
		}
		if err := tx.Create(&powerRow).Error; err != nil {
			return err
		}
		planRow := planModel(in.Plan, in.Now)
		if err := tx.Create(&planRow).Error; err != nil {
			return err
		}
		for _, step := range in.Plan.Steps {
			stepRow := stepModel(in.Plan.ID, step)
			if err := tx.Create(&stepRow).Error; err != nil {
				return err
			}
			for _, cmd := range step.Commands {
				cmdRow, err := commandModel(step.ID, cmd)
				if err != nil {
					return err
				}
				if err := tx.Create(&cmdRow).Error; err != nil {
					return err
				}
			}
		}
		if in.Plan.Status == plan.StatusRejected {
			return nil
		}
		ok, err := writeCooldown(tx, policy.CooldownKey{
			TenantID:  in.Plan.TenantID,
			PolicyID:  in.Plan.PolicyID,
			Direction: in.Direction,
		}, in.Now, in.CooldownUntil, in.Objective.ID, in.Plan.ID, true)
		if err != nil {
			return err
		}
		if !ok {
			return plan.ErrCooldownHeld
		}
		for _, step := range in.Plan.Steps {
			if step.Status != plan.StatusReady {
				continue
			}
			out := OutboxModel{
				ID:          r.newID(),
				TenantID:    in.Plan.TenantID,
				PlanID:      in.Plan.ID,
				StepID:      step.ID,
				AvailableAt: step.ExecuteAt.UTC(),
				Status:      outboxPending,
				LeaseToken:  0,
				CreatedAt:   in.Now.UTC(),
				UpdatedAt:   in.Now.UTC(),
			}
			if err := tx.Create(&out).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func validateSave(in plan.SaveInput) error {
	if in.Objective == nil || in.Plan == nil {
		return fmt.Errorf("plan: objective and plan are required")
	}
	if err := in.Objective.Validate(); err != nil {
		return err
	}
	if err := in.Plan.Validate(); err != nil {
		return err
	}
	if in.Now.IsZero() {
		return fmt.Errorf("plan: now is required")
	}
	if in.Objective.ID != in.Plan.ObjectiveID || in.Objective.TenantID != in.Plan.TenantID {
		return fmt.Errorf("plan: objective and plan identity differ")
	}
	if in.Objective.PolicyID != in.Plan.PolicyID || in.Objective.PolicyVersion != in.Plan.PolicyVersion {
		return fmt.Errorf("plan: policy reference differs from the objective")
	}
	if !in.Objective.Window.Start.Equal(in.Plan.Window.Start) || !in.Objective.Window.End.Equal(in.Plan.Window.End) {
		return fmt.Errorf("plan: window differs from the objective")
	}
	if in.Plan.Status == plan.StatusRejected {
		return nil
	}
	if in.Plan.PolicyID == "" {
		return fmt.Errorf("plan: policy_id is required to claim cooldown")
	}
	if err := in.Direction.Validate(); err != nil {
		return err
	}
	if !in.CooldownUntil.After(in.Now) {
		return fmt.Errorf("plan: cooldown until must be after now")
	}
	return nil
}

func (r *PlanRepository) ClaimDue(ctx context.Context, workerID string, now time.Time, lease time.Duration) (claim plan.Claim, err error) {
	_, deferLog := logging.WhenDB(ctx, "PlanRepository.ClaimDue", workerID)
	defer func() {
		logged := err
		if errors.Is(logged, plan.ErrNoneDue) {
			logged = nil
		}
		deferLog(claim.StepID, &logged)
	}()
	if workerID == "" || lease <= 0 || now.IsZero() {
		return plan.Claim{}, fmt.Errorf("plan: worker, lease, and now are required")
	}
	for i := 0; i < 5; i++ {
		claim, err = r.claimOnce(ctx, workerID, now, lease)
		if !errors.Is(err, errClaimContended) {
			return claim, err
		}
	}
	return plan.Claim{}, fmt.Errorf("plan: claim contended")
}

func (r *PlanRepository) claimOnce(ctx context.Context, workerID string, now time.Time, lease time.Duration) (plan.Claim, error) {
	var claim plan.Claim
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []OutboxModel
		if err := tx.Where("status IN ?", []string{outboxPending, outboxClaimed}).
			Order("available_at ASC, id ASC").
			Find(&rows).Error; err != nil {
			return err
		}
		row, ok := firstDue(rows, now)
		if !ok {
			return plan.ErrNoneDue
		}
		nextToken := row.LeaseToken + 1
		until := now.Add(lease).UTC()
		res := tx.Model(&OutboxModel{}).
			Where("id = ? AND lease_token = ? AND status IN ?", row.ID, row.LeaseToken, []string{outboxPending, outboxClaimed}).
			Updates(map[string]any{
				"status":      outboxClaimed,
				"claimed_by":  workerID,
				"lease_until": until,
				"lease_token": nextToken,
				"updated_at":  now.UTC(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errClaimContended
		}
		loaded, err := loadClaim(tx, r.newID(), row, workerID, nextToken, now)
		if err != nil {
			return err
		}
		claim = loaded
		return nil
	})
	return claim, err
}

func firstDue(rows []OutboxModel, now time.Time) (OutboxModel, bool) {
	for _, row := range rows {
		if row.AvailableAt.After(now) {
			continue
		}
		if row.LeaseUntil != nil && row.LeaseUntil.After(now) {
			continue
		}
		return row, true
	}
	return OutboxModel{}, false
}

func loadClaim(tx *gorm.DB, executionID string, out OutboxModel, workerID string, token int64, now time.Time) (plan.Claim, error) {
	var step StepModel
	if err := tx.Where("id = ?", out.StepID).Take(&step).Error; err != nil {
		return plan.Claim{}, err
	}
	var planRow PlanModel
	if err := tx.Where("id = ?", out.PlanID).Take(&planRow).Error; err != nil {
		return plan.Claim{}, err
	}
	var obj ObjectiveModel
	if err := tx.Where("id = ?", planRow.ObjectiveID).Take(&obj).Error; err != nil {
		return plan.Claim{}, err
	}
	var cmds []CommandModel
	if err := tx.Where("step_id = ?", step.ID).Order("cu_code ASC, id ASC").Find(&cmds).Error; err != nil {
		return plan.Claim{}, err
	}
	if err := tx.Model(&ExecutionModel{}).
		Where("step_id = ? AND status = ?", step.ID, plan.ExecutionClaimed).
		Updates(map[string]any{
			"status":     plan.ExecutionUncertain,
			"error":      "lease expired before dispatch finished",
			"updated_at": now.UTC(),
		}).Error; err != nil {
		return plan.Claim{}, err
	}
	var maxAttempt int
	if err := tx.Model(&ExecutionModel{}).
		Where("step_id = ?", step.ID).
		Select("COALESCE(MAX(attempt), 0)").
		Scan(&maxAttempt).Error; err != nil {
		return plan.Claim{}, err
	}
	key := plan.StepIdempotencyKey(step.ID)
	if maxAttempt > 0 {
		var first ExecutionModel
		if err := tx.Where("step_id = ? AND attempt = ?", step.ID, 1).Take(&first).Error; err != nil {
			return plan.Claim{}, err
		}
		if first.IdempotencyKey != "" {
			key = first.IdempotencyKey
		}
	}
	execution := ExecutionModel{
		ID:             executionID,
		TenantID:       out.TenantID,
		PlanID:         out.PlanID,
		StepID:         step.ID,
		IdempotencyKey: key,
		Attempt:        maxAttempt + 1,
		Status:         plan.ExecutionClaimed,
		ClaimedBy:      workerID,
		CreatedAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
	}
	if err := tx.Create(&execution).Error; err != nil {
		return plan.Claim{}, err
	}
	commands := make([]plan.PlannedCommand, len(cmds))
	for i := range cmds {
		commands[i] = commandFromModel(cmds[i])
	}
	policyID := ""
	if planRow.PolicyID != nil {
		policyID = *planRow.PolicyID
	}
	return plan.Claim{
		WorkerID:         workerID,
		OutboxID:         out.ID,
		LeaseToken:       token,
		ExecutionID:      execution.ID,
		Attempt:          execution.Attempt,
		IdempotencyKey:   key,
		TenantID:         out.TenantID,
		PlanID:           out.PlanID,
		StepID:           step.ID,
		StepVersion:      step.Version,
		PolicyID:         policyID,
		Scope:            policy.TargetScope{Type: port.ScopeType(obj.ScopeType), ID: obj.ScopeID},
		ResourceRevision: planRow.ResourceRevision,
		Commands:         commands,
	}, nil
}

func (r *PlanRepository) MarkSubmitted(ctx context.Context, claim plan.Claim, taskID string, now time.Time) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PlanRepository.MarkSubmitted", claim.StepID)
	defer func() { deferLog(taskID, &err) }()
	if taskID == "" {
		return fmt.Errorf("plan: dispatch task id is required")
	}
	err = r.finish(ctx, claim, now, func(tx *gorm.DB) error {
		if err := updateExecution(tx, claim, plan.ExecutionSubmitted, taskID, "", now); err != nil {
			return err
		}
		if err := updatePlanStatus(tx, claim.PlanID, plan.StatusReady, plan.StatusSubmitted); err != nil {
			return err
		}
		if err := updateStep(tx, claim, plan.StatusSubmitted, taskID); err != nil {
			return err
		}
		if err := tx.Model(&CommandModel{}).Where("step_id = ?", claim.StepID).
			Update("dispatch_task_id", taskID).Error; err != nil {
			return err
		}
		return releaseOutbox(tx, claim, outboxDone, nil, now)
	})
	return err
}

func (r *PlanRepository) MarkUncertain(ctx context.Context, claim plan.Claim, cause string, now time.Time, retryAt time.Time) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PlanRepository.MarkUncertain", claim.StepID)
	defer func() { deferLog(nil, &err) }()
	if retryAt.IsZero() {
		return fmt.Errorf("plan: retry time is required")
	}
	err = r.finish(ctx, claim, now, func(tx *gorm.DB) error {
		if err := updateExecution(tx, claim, plan.ExecutionUncertain, "", clip(cause), now); err != nil {
			return err
		}
		return releaseOutbox(tx, claim, outboxPending, &retryAt, now)
	})
	return err
}

func (r *PlanRepository) MarkStale(ctx context.Context, claim plan.Claim, cause string, now time.Time) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PlanRepository.MarkStale", claim.StepID)
	defer func() { deferLog(nil, &err) }()
	err = r.finish(ctx, claim, now, func(tx *gorm.DB) error {
		if err := updateExecution(tx, claim, plan.ExecutionStale, "", clip(cause), now); err != nil {
			return err
		}
		if err := updatePlanStatus(tx, claim.PlanID, plan.StatusReady, plan.StatusStale); err != nil {
			return err
		}
		if err := updateStep(tx, claim, plan.StatusStale, ""); err != nil {
			return err
		}
		return releaseOutbox(tx, claim, outboxStale, nil, now)
	})
	return err
}

func (r *PlanRepository) MarkFailed(ctx context.Context, claim plan.Claim, cause string, now time.Time) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PlanRepository.MarkFailed", claim.StepID)
	defer func() { deferLog(nil, &err) }()
	err = r.finish(ctx, claim, now, func(tx *gorm.DB) error {
		if err := updateExecution(tx, claim, plan.ExecutionFailed, "", clip(cause), now); err != nil {
			return err
		}
		return releaseOutbox(tx, claim, outboxFailed, nil, now)
	})
	return err
}

func (r *PlanRepository) finish(ctx context.Context, claim plan.Claim, now time.Time, apply func(tx *gorm.DB) error) error {
	if claim.OutboxID == "" || claim.WorkerID == "" || claim.LeaseToken <= 0 || now.IsZero() {
		return plan.ErrLeaseLost
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row OutboxModel
		err := tx.Where("id = ?", claim.OutboxID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return plan.ErrLeaseLost
		}
		if err != nil {
			return err
		}
		if row.LeaseToken != claim.LeaseToken || row.ClaimedBy != claim.WorkerID || row.Status != outboxClaimed {
			return plan.ErrLeaseLost
		}
		if row.LeaseUntil == nil || !row.LeaseUntil.After(now) {
			return plan.ErrLeaseLost
		}
		return apply(tx)
	})
}

func updateExecution(tx *gorm.DB, claim plan.Claim, status, taskID, cause string, now time.Time) error {
	res := tx.Model(&ExecutionModel{}).
		Where("id = ? AND step_id = ? AND status = ?", claim.ExecutionID, claim.StepID, plan.ExecutionClaimed).
		Updates(map[string]any{
			"status":           status,
			"dispatch_task_id": taskID,
			"error":            cause,
			"updated_at":       now.UTC(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return plan.ErrLeaseLost
	}
	return nil
}

func updatePlanStatus(tx *gorm.DB, planID string, from, to plan.Status) error {
	res := tx.Model(&PlanModel{}).
		Where("id = ? AND status = ?", planID, string(from)).
		Update("status", string(to))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return plan.ErrLeaseLost
	}
	return nil
}

func updateStep(tx *gorm.DB, claim plan.Claim, status plan.Status, taskID string) error {
	res := tx.Model(&StepModel{}).
		Where("id = ? AND version = ?", claim.StepID, claim.StepVersion).
		Updates(map[string]any{
			"status":           string(status),
			"version":          claim.StepVersion + 1,
			"dispatch_task_id": taskID,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return plan.ErrLeaseLost
	}
	return nil
}

func releaseOutbox(tx *gorm.DB, claim plan.Claim, status string, availableAt *time.Time, now time.Time) error {
	updates := map[string]any{
		"status":      status,
		"claimed_by":  "",
		"lease_until": gorm.Expr("NULL"),
		"updated_at":  now.UTC(),
	}
	if availableAt != nil {
		updates["available_at"] = availableAt.UTC()
	}
	res := tx.Model(&OutboxModel{}).
		Where("id = ? AND lease_token = ? AND claimed_by = ?", claim.OutboxID, claim.LeaseToken, claim.WorkerID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return plan.ErrLeaseLost
	}
	return nil
}

func clip(s string) string {
	const max = 1024
	if len(s) <= max {
		return s
	}
	return s[:max]
}
