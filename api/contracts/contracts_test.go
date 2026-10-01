package contracts

import (
	"testing"
)

func TestParseMetricID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "known", raw: string(MetricElectricalActivePower), wantErr: false},
		{name: "legacy alias", raw: "soc", wantErr: true},
		{name: "unknown but well formed", raw: "electrical.voltage.v1", wantErr: true},
		{name: "uppercase", raw: "Electrical.active_power.v1", wantErr: true},
		{name: "version zero", raw: "electrical.active_power.v0", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseMetricID(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMetricID(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
		})
	}
}

func TestRegistryDescriptors(t *testing.T) {
	t.Parallel()

	seenMetrics := make(map[MetricID]struct{})
	for _, descriptor := range AllMetrics() {
		if _, duplicate := seenMetrics[descriptor.ID]; duplicate {
			t.Fatalf("duplicate metric descriptor %q", descriptor.ID)
		}
		seenMetrics[descriptor.ID] = struct{}{}
		if descriptor.DisplayName == "" || descriptor.CanonicalUnit == "" {
			t.Fatalf("incomplete metric descriptor: %+v", descriptor)
		}
		if !descriptor.Readable && !descriptor.Writable {
			t.Fatalf("metric %q is neither readable nor writable", descriptor.ID)
		}
	}

	seenCapabilities := make(map[CapabilityID]struct{})
	for _, descriptor := range AllCapabilities() {
		if _, duplicate := seenCapabilities[descriptor.ID]; duplicate {
			t.Fatalf("duplicate capability descriptor %q", descriptor.ID)
		}
		seenCapabilities[descriptor.ID] = struct{}{}
		if descriptor.DisplayName == "" || descriptor.SchemaVersion < 1 {
			t.Fatalf("incomplete capability descriptor: %+v", descriptor)
		}
	}
}

func TestValidateCapabilitySpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      CapabilityID
		version int
		spec    string
		wantErr bool
	}{
		{
			name:    "energy storage",
			id:      CapabilityEnergyStorage,
			version: 1,
			spec:    `{"usable_energy_kwh":200,"max_charge_power_kw":50,"max_discharge_power_kw":50}`,
		},
		{
			name:    "unknown field",
			id:      CapabilityEnergyStorage,
			version: 1,
			spec:    `{"usable_energy_kwh":200,"max_charge_power_kw":50,"max_discharge_power_kw":50,"vendor":"x"}`,
			wantErr: true,
		},
		{
			name:    "missing required positive field",
			id:      CapabilityEnergyStorage,
			version: 1,
			spec:    `{"usable_energy_kwh":200,"max_charge_power_kw":50}`,
			wantErr: true,
		},
		{
			name:    "wrong version",
			id:      CapabilityEnergyStorage,
			version: 2,
			spec:    `{"usable_energy_kwh":200,"max_charge_power_kw":50,"max_discharge_power_kw":50}`,
			wantErr: true,
		},
		{
			name:    "generation",
			id:      CapabilityPowerGeneration,
			version: 1,
			spec:    `{"rated_power_kw":500,"min_power_kw":0,"max_ramp_kw_per_minute":100}`,
		},
		{
			name:    "flexibility must do something",
			id:      CapabilityLoadFlexibility,
			version: 1,
			spec:    `{"max_reduction_kw":0,"max_increase_kw":0}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateCapabilitySpec(tt.id, tt.version, []byte(tt.spec))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateCapabilitySpec() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
