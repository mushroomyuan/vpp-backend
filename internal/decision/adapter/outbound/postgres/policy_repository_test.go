package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
)

func TestPolicyRepository_NameLockAndSoftDelete(t *testing.T) {
	repo := newSQLiteRepo(t)
	ctx := context.Background()

	first := mustPolicy(t, "11111111-1111-4111-8111-111111111111", "tenant-1", "asset soc")
	if err := repo.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	dup := mustPolicy(t, "22222222-2222-4222-8222-222222222222", "tenant-1", "asset soc")
	if err := repo.Create(ctx, dup); !errors.Is(err, policy.ErrNameTaken) {
		t.Fatalf("duplicate name err = %v", err)
	}
	other := mustPolicy(t, "33333333-3333-4333-8333-333333333333", "tenant-2", "asset soc")
	if err := repo.Create(ctx, other); err != nil {
		t.Fatalf("other tenant: %v", err)
	}

	loaded, err := repo.FindByID(ctx, first.TenantID, first.ID)
	if err != nil || loaded.SOC.ChargePowerKW != 100 || loaded.Cooldown != 2*time.Minute {
		t.Fatalf("loaded = %+v err %v", loaded, err)
	}

	stale := loaded.Clone()
	stale.Name = "stale"
	stale.Version = loaded.Version + 2
	if err := repo.Update(ctx, stale, loaded.Version+1); !errors.Is(err, policy.ErrVersionConflict) {
		t.Fatalf("stale update err = %v", err)
	}

	next := loaded.Clone()
	next.Name = "renamed"
	next.Version = loaded.Version + 1
	next.Enabled = true
	if err := repo.Update(ctx, next, loaded.Version); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, next, loaded.Version); !errors.Is(err, policy.ErrVersionConflict) {
		t.Fatalf("second write with old version err = %v", err)
	}

	if err := repo.SoftDelete(ctx, next.TenantID, next.ID, "admin", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindByID(ctx, next.TenantID, next.ID); !errors.Is(err, policy.ErrNotFound) {
		t.Fatalf("deleted find err = %v", err)
	}
	reused := mustPolicy(t, "44444444-4444-4444-8444-444444444444", "tenant-1", "asset soc")
	if err := repo.Create(ctx, reused); err != nil {
		t.Fatalf("reuse name after delete: %v", err)
	}
	listed, err := repo.List(ctx, policy.ListFilter{TenantID: "tenant-1", EnabledOnly: true})
	if err != nil || len(listed) != 0 {
		t.Fatalf("enabled list = %d err %v", len(listed), err)
	}

	enabled := other.Clone()
	enabled.Enabled = true
	enabled.Version = other.Version + 1
	if err := repo.Update(ctx, enabled, other.Version); err != nil {
		t.Fatal(err)
	}
	all, err := repo.ListEnabled(ctx)
	if err != nil || len(all) != 1 || all[0].ID != other.ID || all[0].TenantID != "tenant-2" {
		t.Fatalf("ListEnabled = %+v err %v", all, err)
	}
}

func newSQLiteRepo(t *testing.T) *PolicyRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&PolicyModel{}, &SOCThresholdSpecModel{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_decision_policies_tenant_name
		ON decision_policies (tenant_id, name)
		WHERE deleted_at IS NULL
	`).Error; err != nil {
		t.Fatal(err)
	}
	return NewPolicyRepository(platformpostgres.NewPostgresWithDB(db))
}

func mustPolicy(t *testing.T, id, tenantID, name string) *policy.Policy {
	t.Helper()
	p, err := policy.NewPolicy(policy.NewPolicyParams{
		ID:       id,
		TenantID: tenantID,
		Name:     name,
		Kind:     policy.KindSOCThreshold,
		Scope:    policy.TargetScope{Type: port.ScopeAsset, ID: "asset-1"},
		Cooldown: 2 * time.Minute,
		SOC: &policy.SOCThresholdSpec{
			MinSOC: 20, MaxSOC: 90, ChargePowerKW: 100, DischargePowerKW: 80,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	return p
}
