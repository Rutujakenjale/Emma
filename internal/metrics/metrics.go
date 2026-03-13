package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	JobAccepted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "coupon_import_jobs_accepted_total",
		Help: "Total import jobs accepted",
	})
	JobProcessed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "coupon_import_jobs_processed_total",
		Help: "Total import jobs processed successfully",
	})
	JobFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "coupon_import_jobs_failed_total",
		Help: "Total import jobs failed",
	})
	QueueLength = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "coupon_import_queue_length",
		Help: "Current length of the job queue",
	})
	WorkerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "coupon_import_worker_count",
		Help: "Number of active worker goroutines",
	})
	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "coupon_import_http_requests_total",
		Help: "HTTP requests received",
	}, []string{"method", "code"})
	HTTPDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "coupon_import_http_request_duration_seconds",
		Help:    "Histogram of HTTP request durations",
		Buckets: prometheus.DefBuckets,
	})
	DBQueryDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "coupon_import_db_query_duration_seconds",
		Help:    "Histogram of DB query durations",
		Buckets: prometheus.DefBuckets,
	})
	DBErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "coupon_import_db_errors_total",
		Help: "Total DB errors",
	})
)

func init() {
	prometheus.MustRegister(JobAccepted, JobProcessed, JobFailed, QueueLength, WorkerCount, HTTPRequests, HTTPDuration, DBQueryDuration, DBErrors)
}
