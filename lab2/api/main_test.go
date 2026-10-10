package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type safeBuffer struct {
	sync.Mutex
	bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(p)
}
func TestService(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer tp.Shutdown(context.Background())
	var logs safeBuffer
	a := newApplication(slog.New(slog.NewJSONHandler(&logs, nil)))
	server := httptest.NewServer(a.routes())
	defer server.Close()
	a.selfURL = server.URL
	get := func(path string, code int) (string, string) {
		t.Helper()
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != code {
			t.Fatalf("%s: status %d, body %s", path, resp.StatusCode, body)
		}
		return string(body), resp.Header.Get("X-Trace-ID")
	}
	body, _ := get("/health", 200)
	if body != "ok" {
		t.Fatal(body)
	}
	_, failID := get("/fail", 500)
	start := time.Now()
	_, slowID := get("/slow", 200)
	if time.Since(start) < 2*time.Second {
		t.Fatal("slow returned too early")
	}
	body, loadID := get("/load", 200)
	var result struct{ Requested, Succeeded, Failed int }
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if result.Requested != 100 || result.Succeeded != 100 || result.Failed != 0 {
		t.Fatal(body)
	}
	metrics, _ := get("/metrics", 200)
	for _, want := range []string{`http_requests_total{method="GET",route="/health",status="200"} 101`, `http_errors_total{method="GET",route="/fail"} 1`, `http_request_duration_seconds_count{method="GET",route="/slow"} 1`} {
		if !strings.Contains(metrics, want) {
			t.Errorf("missing metric %s", want)
		}
	}
	second, _ := get("/metrics", 200)
	if metrics != second {
		t.Error("scrapes changed application metrics")
	}
	var failed, slow, propagated bool
	for _, span := range recorder.Ended() {
		id := span.SpanContext().TraceID().String()
		if id == failID && span.Status().Code == codes.Error {
			failed = true
		}
		if id == slowID && span.Name() == "slow-op" && span.Parent().IsValid() && span.EndTime().Sub(span.StartTime()) >= 2*time.Second {
			slow = true
		}
		if id == loadID && span.Name() == "GET /health" && span.Parent().IsValid() {
			propagated = true
		}
	}
	if !failed || !slow || !propagated {
		t.Fatalf("spans: failed=%v slow=%v propagated=%v", failed, slow, propagated)
	}
	logs.Lock()
	text := logs.Buffer.String()
	logs.Unlock()
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		id, ok := entry["trace_id"].(string)
		if !ok || len(id) != 32 || id == strings.Repeat("0", 32) {
			t.Fatal("invalid trace_id", entry)
		}
	}
	if !strings.Contains(text, failID) || !strings.Contains(text, slowID) {
		t.Fatal("logs not linked to spans")
	}
}
