package api

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	reqTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "http_requests_total", Help: "Total HTTP requests."},
		[]string{"route", "method", "status"},
	)
	reqLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"route", "method"},
	)
)

func init() {
	prometheus.MustRegister(reqTotal, reqLatency)
}

// Metrics records RED metrics (rate / errors / duration) per request.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched" // avoid unbounded label cardinality from 404s
		}
		reqTotal.WithLabelValues(route, c.Request.Method, strconv.Itoa(c.Writer.Status())).Inc()
		reqLatency.WithLabelValues(route, c.Request.Method).Observe(time.Since(start).Seconds())
	}
}

// prometheusHandler exposes the registry at /metrics.
func prometheusHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) { h.ServeHTTP(c.Writer, c.Request) }
}
