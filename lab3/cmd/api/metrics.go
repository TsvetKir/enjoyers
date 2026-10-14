package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

type apiMetrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func newAPIMetrics() *apiMetrics {
	m := &apiMetrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "shop_http_requests_total", Help: "Completed API requests by route, method and HTTP code."}, []string{"route", "method", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "shop_http_request_duration_seconds", Help: "API request duration in seconds.", Buckets: prometheus.DefBuckets}, []string{"route", "method"}),
	}
	m.registry.MustRegister(m.requests, m.duration, collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}

func (m *apiMetrics) handler(next http.Handler) http.Handler {
	// Fixed route labels prevent unbounded series from arbitrary URLs.
	handlers := map[string]http.Handler{}
	for _, route := range []string{"/health", "/order", "/orders", "other"} {
		handlers[route] = promhttp.InstrumentHandlerCounter(m.requests.MustCurryWith(prometheus.Labels{"route": route}),
			promhttp.InstrumentHandlerDuration(m.duration.MustCurryWith(prometheus.Labels{"route": route}), next))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		route := r.URL.Path
		if _, ok := handlers[route]; !ok {
			route = "other"
		}
		handlers[route].ServeHTTP(w, r)
	})
}
