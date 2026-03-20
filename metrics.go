package tn3270

import "go.k6.io/k6/metrics"

type tn3270Metrics struct {
	ConnectDuration *metrics.Metric
	SendDuration    *metrics.Metric
	WaitDuration    *metrics.Metric
	Errors          *metrics.Metric
	Screens         *metrics.Metric
}

func registerMetrics(registry *metrics.Registry) (*tn3270Metrics, error) {
	m := &tn3270Metrics{}
	var err error

	if m.ConnectDuration, err = registry.NewMetric("tn3270_connect_duration", metrics.Trend, metrics.Time); err != nil {
		return nil, err
	}
	if m.SendDuration, err = registry.NewMetric("tn3270_send_duration", metrics.Trend, metrics.Time); err != nil {
		return nil, err
	}
	if m.WaitDuration, err = registry.NewMetric("tn3270_wait_duration", metrics.Trend, metrics.Time); err != nil {
		return nil, err
	}
	if m.Errors, err = registry.NewMetric("tn3270_errors", metrics.Counter, metrics.Default); err != nil {
		return nil, err
	}
	if m.Screens, err = registry.NewMetric("tn3270_screens", metrics.Counter, metrics.Default); err != nil {
		return nil, err
	}

	return m, nil
}
