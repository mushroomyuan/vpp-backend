package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
)

// CooldownStore is the Postgres cooldown. TryTrigger competes; at most one
// caller acquires a direction whose window has not expired.
type CooldownStore struct {
	db *gorm.DB
}

// NewCooldownStore binds the store to the decision database.
func NewCooldownStore(pg *platformpostgres.Postgres) *CooldownStore {
	if pg == nil {
		panic("NewCooldownStore: postgres is required")
	}
	return &CooldownStore{db: pg.DB()}
}

var _ policy.CooldownStore = (*CooldownStore)(nil)

// CoolingDown reports whether now is still before the exclusive end of the window.
func (s *CooldownStore) CoolingDown(ctx context.Context, key policy.CooldownKey, now time.Time) (bool, error) {
	key = normalizeCooldownKey(key)
	if err := key.Validate(); err != nil {
		return false, err
	}
	var row CooldownModel
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND policy_id = ? AND direction = ?", key.TenantID, key.PolicyID, string(key.Direction)).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return now.Before(row.CooldownUntil), nil
}

// MarkTriggered stores the window even when one is already active.
// The plan transaction uses TryTrigger, which loses when the window is still open.
func (s *CooldownStore) MarkTriggered(ctx context.Context, key policy.CooldownKey, until time.Time) error {
	key = normalizeCooldownKey(key)
	if err := key.Validate(); err != nil {
		return err
	}
	if until.IsZero() {
		return policy.Invalid(fmt.Errorf("policy: cooldown until is required"))
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := writeCooldown(tx, key, until, until, "", "", false)
		return err
	})
}

// TryTrigger stores until when the key is free at now.
// acquired is false when an existing window still covers now.
func (s *CooldownStore) TryTrigger(ctx context.Context, key policy.CooldownKey, now, until time.Time) (bool, error) {
	var acquired bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ok, err := writeCooldown(tx, key, now, until, "", "", true)
		acquired = ok
		return err
	})
	return acquired, err
}

func normalizeCooldownKey(key policy.CooldownKey) policy.CooldownKey {
	key.TenantID = strings.TrimSpace(key.TenantID)
	key.PolicyID = strings.TrimSpace(key.PolicyID)
	return key
}

// writeCooldown inserts or replaces the window.
// onlyIfExpired makes a live window return false without changing it.
// Time is compared in Go. The version column is the compare-and-swap, so sqlite
// text timestamps and Postgres timestamptz do not have to sort the same way.
func writeCooldown(tx *gorm.DB, key policy.CooldownKey, now, until time.Time, objectiveID, planID string, onlyIfExpired bool) (bool, error) {
	key = normalizeCooldownKey(key)
	if err := key.Validate(); err != nil {
		return false, err
	}
	if !until.After(now) && onlyIfExpired {
		return false, fmt.Errorf("plan: cooldown until must be after now")
	}
	if until.IsZero() {
		return false, fmt.Errorf("plan: cooldown until is required")
	}
	for attempt := 0; attempt < 3; attempt++ {
		var row CooldownModel
		err := tx.Where("tenant_id = ? AND policy_id = ? AND direction = ?", key.TenantID, key.PolicyID, string(key.Direction)).
			Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.SavePoint("cooldown_claim").Error; err != nil {
				return false, err
			}
			created := CooldownModel{
				TenantID:        key.TenantID,
				PolicyID:        key.PolicyID,
				Direction:       string(key.Direction),
				CooldownUntil:   until.UTC(),
				LastObjectiveID: nullableID(objectiveID),
				LastPlanID:      nullableID(planID),
				Version:         1,
				UpdatedAt:       now.UTC(),
			}
			if err := tx.Create(&created).Error; err != nil {
				if rb := tx.RollbackTo("cooldown_claim").Error; rb != nil {
					return false, rb
				}
				if isUniqueViolation(err) {
					continue
				}
				return false, err
			}
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if onlyIfExpired && now.Before(row.CooldownUntil) {
			return false, nil
		}
		res := tx.Model(&CooldownModel{}).
			Where("tenant_id = ? AND policy_id = ? AND direction = ? AND version = ?", key.TenantID, key.PolicyID, string(key.Direction), row.Version).
			Updates(map[string]any{
				"cooldown_until":    until.UTC(),
				"last_objective_id": nullableID(objectiveID),
				"last_plan_id":      nullableID(planID),
				"version":           row.Version + 1,
				"updated_at":        now.UTC(),
			})
		if res.Error != nil {
			return false, res.Error
		}
		if res.RowsAffected == 1 {
			return true, nil
		}
	}
	return false, fmt.Errorf("plan: cooldown claim contended")
}
