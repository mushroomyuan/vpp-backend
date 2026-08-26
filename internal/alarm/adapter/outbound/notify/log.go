package notify

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

// LogNotifier is the v1 Notifier: structured log only, no mail/SMS.
type LogNotifier struct{}

func NewLogNotifier() *LogNotifier { return &LogNotifier{} }

var _ port.Notifier = (*LogNotifier)(nil)

func (n *LogNotifier) Notify(ctx context.Context, alarm *model.Alarm) error {
	if alarm == nil {
		return nil
	}
	logging.Infof(ctx, logrus.Fields{
		"component":   "AlarmNotifier",
		"alarm_id":    alarm.ID,
		"tenant_id":   alarm.TenantID,
		"source":      alarm.Source,
		"severity":    alarm.Severity,
		"fingerprint": alarm.Fingerprint,
		"title":       alarm.Title,
	}, "alarm opened")
	return nil
}
