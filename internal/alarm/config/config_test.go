package config

import (
	"testing"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/options"
)

func TestCreateFromOptions_Defaults(t *testing.T) {
	t.Parallel()
	opts := options.NewOptions()
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("%v", errs)
	}
	cfg, err := CreateFromOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8087" || cfg.MetricsAddr != ":9107" {
		t.Fatalf("addrs %+v", cfg)
	}
	if cfg.TrustProxyHeaders || cfg.Authz.Enabled {
		t.Fatal("auth must be off by default")
	}
	if cfg.Rules.DispatchTaskFailed.Severity != model.SeverityCritical {
		t.Fatalf("rules %+v", cfg.Rules)
	}
	if cfg.Kafka.DispatchGroupID != "vpp-alarm-dispatch-events" {
		t.Fatalf("kafka %+v", cfg.Kafka)
	}
}

func TestCreateFromOptions_SOEWhitelist(t *testing.T) {
	t.Parallel()
	opts := options.NewOptions()
	opts.Alarm.Rules.SOEDiscreteChange.MetricIDs = []string{"electrical.active_power.v1"}
	opts.Alarm.Rules.SOEDiscreteChange.Severity = "info"
	cfg, err := CreateFromOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules.SOEDiscreteChange.Severity != model.SeverityInfo {
		t.Fatalf("%+v", cfg.Rules.SOEDiscreteChange)
	}
	if len(cfg.Rules.SOEDiscreteChange.MetricIDs) != 1 {
		t.Fatal(cfg.Rules.SOEDiscreteChange.MetricIDs)
	}
	if cfg.Rules.SOEQualityBad.Severity != model.SeverityCritical || !cfg.Rules.SOERecovery.Enabled {
		t.Fatalf("seed rules %+v", cfg.Rules)
	}
}

func TestCreateFromOptions_RejectsVendorMetricID(t *testing.T) {
	t.Parallel()
	opts := options.NewOptions()
	opts.Alarm.Rules.SOEDiscreteChange.MetricIDs = []string{"switch_pos"}
	if _, err := CreateFromOptions(opts); err == nil {
		t.Fatal("vendor point name must fail rule validation")
	}
}
