package authz

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// Syncer periodically pulls Casdoor permissions into a Checker (B1).
type Syncer struct {
	src     PermissionSource
	checker *Checker
	cfg     Config
	log     logrus.FieldLogger
}

// NewSyncer wires a permission source to checker. cfg should match the checker.
func NewSyncer(src PermissionSource, checker *Checker, cfg Config) *Syncer {
	cfg = cfg.withDefaults()
	return &Syncer{
		src:     src,
		checker: checker,
		cfg:     cfg,
		log:     logrus.WithField("component", "authz.syncer"),
	}
}

// SyncOnce fetches permissions and replaces local policies on success.
func (s *Syncer) SyncOnce(ctx context.Context) error {
	perms, err := s.src.FetchPermissions(ctx, s.cfg.Owner)
	if err != nil {
		return err
	}
	rules := PoliciesFromPermissions(perms, s.cfg.ModelFilter)
	now := time.Now().UTC()
	if err := s.checker.ReplacePolicies(rules, now); err != nil {
		return err
	}
	s.log.WithFields(logrus.Fields{
		"owner":    s.cfg.Owner,
		"rules":    len(rules),
		"perms":    len(perms),
		"syncedAt": now,
	}).Info("authz policy sync ok")
	return nil
}

// Run syncs immediately, then on interval until ctx is cancelled.
func (s *Syncer) Run(ctx context.Context) error {
	if err := s.SyncOnce(ctx); err != nil {
		s.log.WithError(err).Error("authz initial policy sync failed")
		// Continue into the loop so later intervals can recover.
	}
	t := time.NewTicker(s.cfg.SyncInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := s.SyncOnce(ctx); err != nil {
				s.log.WithError(err).Error("authz policy sync failed")
			}
		}
	}
}

// MustSyncOnce is like SyncOnce but wraps errors for startup fail-fast callers.
func (s *Syncer) MustSyncOnce(ctx context.Context) error {
	if err := s.SyncOnce(ctx); err != nil {
		return fmt.Errorf("authz sync: %w", err)
	}
	return nil
}
