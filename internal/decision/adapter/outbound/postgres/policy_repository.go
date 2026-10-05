package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
)

// PolicyRepository is the GORM store for decision_policies and decision_soc_threshold_specs.
type PolicyRepository struct {
	db *gorm.DB
}

// NewPolicyRepository binds the repository to the shared pool.
func NewPolicyRepository(pg *platformpostgres.Postgres) *PolicyRepository {
	if pg == nil {
		panic("NewPolicyRepository: postgres is required")
	}
	return &PolicyRepository{db: pg.DB()}
}

var _ policy.Repository = (*PolicyRepository)(nil)

func (r *PolicyRepository) Create(ctx context.Context, p *policy.Policy) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PolicyRepository.Create", p.ID)
	defer func() { deferLog(p.ID, &err) }()
	if err = p.Validate(); err != nil {
		return err
	}
	row, spec := policyToModels(p)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return mapWriteError(err)
		}
		if err := tx.Create(&spec).Error; err != nil {
			return mapWriteError(err)
		}
		return nil
	})
	return err
}

func (r *PolicyRepository) Update(ctx context.Context, p *policy.Policy, expectedVersion int64) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PolicyRepository.Update", p.ID)
	defer func() { deferLog(nil, &err) }()
	if err = p.Validate(); err != nil {
		return err
	}
	if p.Version != expectedVersion+1 {
		return fmt.Errorf("policy: stored version must advance from %d to %d", expectedVersion, expectedVersion+1)
	}
	row, spec := policyToModels(p)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&PolicyModel{}).
			Where("id = ? AND tenant_id = ? AND version = ? AND deleted_at IS NULL", row.ID, row.TenantID, expectedVersion).
			Updates(map[string]any{
				"name":        row.Name,
				"scope_type":  row.ScopeType,
				"scope_id":    row.ScopeID,
				"enabled":     row.Enabled,
				"cooldown_ns": row.CooldownNs,
				"version":     row.Version,
				"updated_by":  row.UpdatedBy,
				"updated_at":  row.UpdatedAt,
			})
		if result.Error != nil {
			return mapWriteError(result.Error)
		}
		if result.RowsAffected == 0 {
			return r.conflictOrMissing(tx, row.TenantID, row.ID)
		}
		specResult := tx.Model(&SOCThresholdSpecModel{}).
			Where("policy_id = ?", row.ID).
			Updates(map[string]any{
				"min_soc":            spec.MinSOC,
				"max_soc":            spec.MaxSOC,
				"charge_power_kw":    spec.ChargePowerKW,
				"discharge_power_kw": spec.DischargePowerKW,
			})
		if specResult.Error != nil {
			return specResult.Error
		}
		if specResult.RowsAffected == 0 {
			return fmt.Errorf("policy %s is missing its soc spec", row.ID)
		}
		return nil
	})
	return err
}

func (r *PolicyRepository) SoftDelete(ctx context.Context, tenantID, id, actor string, at time.Time) (err error) {
	_, deferLog := logging.WhenDB(ctx, "PolicyRepository.SoftDelete", id)
	defer func() { deferLog(nil, &err) }()
	result := r.db.WithContext(ctx).Model(&PolicyModel{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Updates(map[string]any{
			"deleted_at": at,
			"deleted_by": actor,
			"updated_at": at,
			"updated_by": actor,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return policy.ErrNotFound
	}
	return nil
}

func (r *PolicyRepository) FindByID(ctx context.Context, tenantID, id string) (p *policy.Policy, err error) {
	_, deferLog := logging.WhenDB(ctx, "PolicyRepository.FindByID", id)
	defer func() { deferLog(id, &err) }()
	var row PolicyModel
	err = r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantID, id).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, policy.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var spec SOCThresholdSpecModel
	err = r.db.WithContext(ctx).Where("policy_id = ?", row.ID).First(&spec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("policy %s is missing its soc spec", id)
	}
	if err != nil {
		return nil, err
	}
	return modelsToPolicy(row, spec)
}

func (r *PolicyRepository) List(ctx context.Context, filter policy.ListFilter) (out []*policy.Policy, err error) {
	_, deferLog := logging.WhenDB(ctx, "PolicyRepository.List", filter.TenantID)
	defer func() { deferLog(len(out), &err) }()
	if filter.TenantID == "" {
		return nil, policy.Invalid(fmt.Errorf("policy: tenant_id is required"))
	}
	query := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL", filter.TenantID)
	if filter.EnabledOnly {
		query = query.Where("enabled = ?", true)
	}
	var rows []PolicyModel
	if err = query.Order("name ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out, err = r.policiesFromRows(ctx, rows)
	return out, err
}

func (r *PolicyRepository) ListEnabled(ctx context.Context) (out []*policy.Policy, err error) {
	_, deferLog := logging.WhenDB(ctx, "PolicyRepository.ListEnabled", nil)
	defer func() { deferLog(len(out), &err) }()
	var rows []PolicyModel
	if err = r.db.WithContext(ctx).
		Where("enabled = ? AND deleted_at IS NULL", true).
		Order("tenant_id ASC, name ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return r.policiesFromRows(ctx, rows)
}

func (r *PolicyRepository) policiesFromRows(ctx context.Context, rows []PolicyModel) ([]*policy.Policy, error) {
	if len(rows) == 0 {
		return []*policy.Policy{}, nil
	}
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	var specs []SOCThresholdSpecModel
	if err := r.db.WithContext(ctx).Where("policy_id IN ?", ids).Find(&specs).Error; err != nil {
		return nil, err
	}
	byID := make(map[string]SOCThresholdSpecModel, len(specs))
	for _, spec := range specs {
		byID[spec.PolicyID] = spec
	}
	out := make([]*policy.Policy, 0, len(rows))
	for _, row := range rows {
		spec, ok := byID[row.ID]
		if !ok {
			return nil, fmt.Errorf("policy %s is missing its soc spec", row.ID)
		}
		item, err := modelsToPolicy(row, spec)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (r *PolicyRepository) conflictOrMissing(tx *gorm.DB, tenantID, id string) error {
	var count int64
	if err := tx.Model(&PolicyModel{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return policy.ErrNotFound
	}
	return policy.ErrVersionConflict
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err) {
		return policy.ErrNameTaken
	}
	return err
}

func isUniqueViolation(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}
	// SQLite (tests) reports the partial unique index this way.
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "duplicate key value")
}
