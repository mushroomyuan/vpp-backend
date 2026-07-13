package runtime

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/simulator/device"
	"github.com/mushroomyuan/vpp-backend/simulator/domain"
	"github.com/mushroomyuan/vpp-backend/simulator/fault"
)

// Manager owns all live Device instances.
type Manager struct {
	mu      sync.RWMutex
	devices map[string]domain.Device // key = CUCode
	byExt   map[string]domain.Device // key = ExternalID
	faults  *fault.Engine
	specs   []domain.DeviceSpec
}

func NewManager(faults *fault.Engine) *Manager {
	return &Manager{
		devices: make(map[string]domain.Device),
		byExt:   make(map[string]domain.Device),
		faults:  faults,
	}
}

func (m *Manager) Load(specs []domain.DeviceSpec) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices = make(map[string]domain.Device, len(specs))
	m.byExt = make(map[string]domain.Device, len(specs))
	m.specs = append([]domain.DeviceSpec(nil), specs...)
	for _, spec := range specs {
		d := device.New(spec)
		m.devices[d.CUCode()] = d
		m.byExt[d.ExternalID()] = d
		logrus.Infof("runtime: registered %s", device.Describe(d))
	}
}

func (m *Manager) ReloadFromSpecs() {
	m.mu.RLock()
	specs := append([]domain.DeviceSpec(nil), m.specs...)
	m.mu.RUnlock()
	m.Load(specs)
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.devices)
}

func (m *Manager) GetByCU(cuCode string) (domain.Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.devices[cuCode]
	if !ok {
		return nil, domain.ErrDeviceNotFound
	}
	return d, nil
}

func (m *Manager) GetByExternalID(externalID string) (domain.Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.byExt[externalID]
	if !ok {
		return nil, domain.ErrDeviceNotFound
	}
	return d, nil
}

func (m *Manager) List() []domain.Device {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Device, 0, len(m.devices))
	for _, d := range m.devices {
		out = append(out, d)
	}
	return out
}

func (m *Manager) TickAll(delta time.Duration) {
	m.mu.RLock()
	devs := make([]domain.Device, 0, len(m.devices))
	for _, d := range m.devices {
		devs = append(devs, d)
	}
	m.mu.RUnlock()

	for _, d := range devs {
		if m.faults != nil && m.faults.IsOffline(d.CUCode(), d.ExternalID()) {
			d.SetStatus(domain.StatusOffline)
			continue
		}
		if d.Status() == domain.StatusOffline {
			d.SetStatus(domain.StatusOnline)
		}
		d.Tick(delta)
	}
}

func (m *Manager) Execute(externalID, pointKey string, value float64) error {
	d, err := m.GetByExternalID(externalID)
	if err != nil {
		// also try CUCode
		d, err = m.GetByCU(externalID)
		if err != nil {
			return err
		}
	}
	if m.faults != nil {
		if m.faults.IsOffline(d.CUCode(), d.ExternalID()) {
			d.SetStatus(domain.StatusOffline)
			return domain.ErrDeviceOffline
		}
		if m.faults.ShouldRejectCommand(d.CUCode(), d.ExternalID()) {
			return domain.ErrCommandRejected
		}
	}
	return d.Execute(pointKey, value)
}

func (m *Manager) ResetAll() {
	for _, d := range m.List() {
		d.Reset()
	}
}

// DeviceSummary is a JSON-friendly view for Debug API.
type DeviceSummary struct {
	CUCode     string               `json:"cu_code"`
	ExternalID string               `json:"external_id"`
	Name       string               `json:"name"`
	Type       string               `json:"type"`
	Status     domain.DeviceStatus  `json:"status"`
	Points     []domain.PointValue  `json:"points"`
	Fault      fault.State          `json:"fault"`
}

func (m *Manager) Summaries() []DeviceSummary {
	out := make([]DeviceSummary, 0, m.Count())
	for _, d := range m.List() {
		out = append(out, m.deviceSummary(d))
	}
	return out
}

func (m *Manager) Summary(cuOrExt string) (DeviceSummary, error) {
	d, err := m.GetByCU(cuOrExt)
	if err != nil {
		d, err = m.GetByExternalID(cuOrExt)
		if err != nil {
			return DeviceSummary{}, err
		}
	}
	return m.deviceSummary(d), nil
}

func (m *Manager) deviceSummary(d domain.Device) DeviceSummary {
	var f fault.State
	if m.faults != nil {
		f = m.faults.Lookup(d.CUCode(), d.ExternalID())
	}
	return DeviceSummary{
		CUCode:     d.CUCode(),
		ExternalID: d.ExternalID(),
		Name:       d.Name(),
		Type:       d.Type(),
		Status:     d.Status(),
		Points:     d.Snapshot(),
		Fault:      f,
	}
}
