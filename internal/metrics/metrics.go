// Package metrics defines the crawler's Prometheus collectors.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds every collector the crawler updates.
type Metrics struct {
	// PagesTotal counts finished URLs by terminal status
	// (done, failed, blocked, skipped, dead).
	PagesTotal *prometheus.CounterVec
	// FetchDuration is the HTTP fetch latency in seconds.
	FetchDuration prometheus.Histogram
	// QueueDepth is the number of pending frontier rows.
	QueueDepth prometheus.Gauge
	// Retries counts fetch attempts that were rescheduled after a transient
	// failure.
	Retries prometheus.Counter
	// RobotsBlocked counts URLs skipped because robots.txt disallows them.
	RobotsBlocked prometheus.Counter
	// HostInflight is the number of requests currently in flight, per host.
	HostInflight *prometheus.GaugeVec
	// GateDeferred counts requests pushed back by the per-host politeness gate.
	GateDeferred prometheus.Counter
}

// New creates the collectors and registers them with reg.
func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		PagesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "crawlerx_pages_total",
			Help: "URLs that reached a terminal state, by status.",
		}, []string{"status"}),
		FetchDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "crawlerx_fetch_duration_seconds",
			Help:    "HTTP fetch latency (request start to end of capped body).",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}),
		QueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "crawlerx_queue_depth",
			Help: "Pending URLs in the frontier.",
		}),
		Retries: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "crawlerx_retries_total",
			Help: "Fetches rescheduled after a transient failure.",
		}),
		RobotsBlocked: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "crawlerx_robots_blocked_total",
			Help: "URLs skipped because robots.txt disallows them.",
		}),
		HostInflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "crawlerx_host_inflight",
			Help: "Requests currently in flight, per host.",
		}, []string{"host"}),
		GateDeferred: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "crawlerx_gate_deferred_total",
			Help: "Requests pushed back by the per-host politeness gate.",
		}),
	}

	reg.MustRegister(m.PagesTotal, m.FetchDuration, m.QueueDepth, m.Retries,
		m.RobotsBlocked, m.HostInflight, m.GateDeferred)

	return m
}
