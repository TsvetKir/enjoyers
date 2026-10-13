package main

import (
	"github.com/prometheus/client_golang/prometheus/testutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricsRecordResponsesAndBoundRoutes(t *testing.T) {
	m := newAPIMetrics()
	h := m.handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/order" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	for _, path := range []string{"/orders", "/order", "/random/one", "/random/two", "/metrics"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	for _, tc := range []struct {
		route, code string
		count       float64
	}{
		{"/orders", "200", 1}, {"/order", "500", 1}, {"other", "200", 2},
	} {
		if got := testutil.ToFloat64(m.requests.WithLabelValues(tc.route, "get", tc.code)); got != tc.count {
			t.Errorf("%s count=%v want=%v", tc.route, got, tc.count)
		}
	}
	families, err := m.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == "shop_http_requests_total" && len(f.Metric) != 3 {
			t.Errorf("Unexpected route series: %d", len(f.Metric))
		}
		if f.GetName() == "shop_http_request_duration_seconds" {
			var count uint64
			for _, metric := range f.Metric {
				count += metric.GetHistogram().GetSampleCount()
			}
			if count != 4 {
				t.Errorf("Histogram count=%d want=4", count)
			}
		}
	}
}
