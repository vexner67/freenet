package rest

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type httpMetrics struct {
	requests *prometheus.CounterVec
	seconds  *prometheus.HistogramVec
	events   prometheus.Gauge
}

func newHTTPMetrics(reg prometheus.Registerer) (*httpMetrics, error) {
	m := &httpMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests completed, by route and status code.",
		}, []string{"method", "route", "code"}),
		seconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "How long HTTP requests took, but the event stream.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		events: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gateway_event_streams_open",
			Help: "Event streams (WebSockets) open now.",
		}),
	}

	for _, c := range []prometheus.Collector{m.requests, m.seconds, m.events} {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}

	return m, nil
}

// observe counts requests by the route that served them, so a label never
// holds a nickname or an ID. It must wrap the mux directly: only the
// request the mux gets learns its pattern.
func (m *httpMetrics) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		// "GET /api/v1/users/{nickname}" without the method.
		_, route, ok := strings.Cut(r.Pattern, " ")
		if !ok {
			route = r.Pattern
		}

		m.requests.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Inc()

		// The stream lives as long as its page: its duration would drown
		// the others.
		if r.URL.Path != eventsPath {
			m.seconds.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		}
	})
}
