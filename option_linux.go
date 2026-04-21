//go:build linux
// +build linux

package autopprof

// validate is invoked by Start on linux — the only platform where
// autopprof actually runs.
func (o Option) validate() error {
	// Disable-all is only an error when no user metrics pick up the
	// slack; a user with one or more Metrics can still make the
	// library do meaningful work.
	if o.DisableCPUProf && o.DisableMemProf && o.DisableGoroutineProf && len(o.Metrics) == 0 {
		return ErrDisableAllProfiling
	}
	if o.CPUThreshold < 0 || o.CPUThreshold > 1 {
		return ErrInvalidCPUThreshold
	}
	if o.MemThreshold < 0 || o.MemThreshold > 1 {
		return ErrInvalidMemThreshold
	}
	if o.GoroutineThreshold < 0 {
		return ErrInvalidGoroutineThreshold
	}
	if o.Reporter == nil {
		return ErrNilReporter
	}

	seen := make(map[string]struct{}, len(o.Metrics))
	for _, m := range o.Metrics {
		if err := validateMetric(m); err != nil {
			return err
		}
		name := m.Name()
		if _, reserved := reservedMetricNames.Load(name); reserved {
			return ErrReservedMetricName
		}
		if _, dup := seen[name]; dup {
			return ErrInvalidMetric
		}
		seen[name] = struct{}{}
	}
	return nil
}
