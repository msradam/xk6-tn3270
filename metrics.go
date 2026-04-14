package tn3270

import "go.k6.io/k6/metrics"

type tn3270Metrics struct {
	ConnectDuration *metrics.Metric
	SendDuration    *metrics.Metric
	WaitDuration    *metrics.Metric
	SessionDuration *metrics.Metric
	Errors          *metrics.Metric
	WaitTimeouts    *metrics.Metric
	Screens         *metrics.Metric
	Connects        *metrics.Metric
	Disconnects     *metrics.Metric
	AIDsSent        *metrics.Metric
	BytesIn         *metrics.Metric
	BytesOut        *metrics.Metric
}

func registerMetrics(registry *metrics.Registry) (*tn3270Metrics, error) {
	m := &tn3270Metrics{}
	defs := []struct {
		target **metrics.Metric
		name   string
		typ    metrics.MetricType
		val    metrics.ValueType
	}{
		{&m.ConnectDuration, "tn3270_connect_duration", metrics.Trend, metrics.Time},
		{&m.SendDuration, "tn3270_send_duration", metrics.Trend, metrics.Time},
		{&m.WaitDuration, "tn3270_wait_duration", metrics.Trend, metrics.Time},
		{&m.SessionDuration, "tn3270_session_duration", metrics.Trend, metrics.Time},
		{&m.Errors, "tn3270_errors", metrics.Counter, metrics.Default},
		{&m.WaitTimeouts, "tn3270_wait_timeouts", metrics.Counter, metrics.Default},
		{&m.Screens, "tn3270_screens", metrics.Counter, metrics.Default},
		{&m.Connects, "tn3270_connects", metrics.Counter, metrics.Default},
		{&m.Disconnects, "tn3270_disconnects", metrics.Counter, metrics.Default},
		{&m.AIDsSent, "tn3270_aids_sent", metrics.Counter, metrics.Default},
		{&m.BytesIn, "tn3270_bytes_in", metrics.Counter, metrics.Data},
		{&m.BytesOut, "tn3270_bytes_out", metrics.Counter, metrics.Data},
	}
	for _, d := range defs {
		metric, err := registry.NewMetric(d.name, d.typ, d.val)
		if err != nil {
			return nil, err
		}
		*d.target = metric
	}
	return m, nil
}
