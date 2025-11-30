package simulation

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	tickLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "orbit_tick_latency_seconds",
		Help:    "Time between simulation ticks.",
		Buckets: prometheus.DefBuckets,
	})

	updateDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "orbit_truck_update_duration_seconds",
		Help:    "Duration spent updating an individual truck.",
		Buckets: prometheus.DefBuckets,
	})

	goroutines = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "orbit_goroutine_count",
		Help: "Number of goroutines running in the simulation.",
	}, func() float64 {
		return float64(runtime.NumGoroutine())
	})
)

func init() {
	prometheus.MustRegister(tickLatency, updateDuration, goroutines)
}
