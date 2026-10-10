package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type application struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	failures *prometheus.CounterVec
	duration *prometheus.HistogramVec
	logger   *slog.Logger
	client   *http.Client
	selfURL  string
	loadGate chan struct{}
}

func newApplication(logger *slog.Logger) *application {
	a := &application{
		registry: prometheus.NewRegistry(), logger: logger,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_requests_total", Help: "Completed application requests."}, []string{"method", "route", "status"}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_errors_total", Help: "Completed application requests with 5xx status."}, []string{"method", "route"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_request_duration_seconds", Help: "Application request duration in seconds.", Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 1.5, 2, 2.5, 3, 5, 10}}, []string{"method", "route"}),
		client:   &http.Client{Timeout: 5 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)},
		loadGate: make(chan struct{}, 1),
	}
	a.registry.MustRegister(a.requests, a.failures, a.duration)
	return a
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *responseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Fixed route labels prevent arbitrary URLs from creating unbounded time series.
func (a *application) observe(route string, next http.HandlerFunc) http.Handler {
	return otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w}
		span := trace.SpanFromContext(r.Context())
		w.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
		next(rw, r)
		if rw.status == 0 {
			rw.WriteHeader(http.StatusOK)
		}
		seconds := time.Since(start).Seconds()
		a.requests.WithLabelValues(r.Method, route, strconv.Itoa(rw.status)).Inc()
		a.duration.WithLabelValues(r.Method, route).Observe(seconds)
		level := slog.LevelInfo
		if rw.status >= 500 {
			a.failures.WithLabelValues(r.Method, route).Inc()
			span.SetStatus(codes.Error, http.StatusText(rw.status))
			level = slog.LevelError
		}
		a.logger.Log(r.Context(), level, "http_request", "method", r.Method, "route", route, "status", rw.status, "duration_seconds", seconds, "trace_id", span.SpanContext().TraceID().String())
	}), "GET "+route, otelhttp.WithSpanOptions(trace.WithAttributes(attribute.String("http.route", route))))
}

// health перенесён из lab1/api-service/main.go.
func health(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok")
}

func (a *application) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", a.observe("/health", health))
	mux.Handle("GET /fail", a.observe("/fail", func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		span.RecordError(errors.New("intentional failure"))
		span.SetStatus(codes.Error, "intentional failure")
		http.Error(w, "intentional failure", http.StatusInternalServerError)
	}))
	mux.Handle("GET /slow", a.observe("/slow", func(w http.ResponseWriter, r *http.Request) {
		ctx, span := otel.Tracer("api").Start(r.Context(), "slow-op")
		defer span.End()
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
			fmt.Fprintln(w, "slow operation completed")
		case <-ctx.Done():
			span.RecordError(ctx.Err())
			span.SetStatus(codes.Error, "request cancelled")
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
		}
	}))
	mux.Handle("GET /load", a.observe("/load", a.load))
	// Scraping metrics must not distort the application RED signals.
	mux.Handle("GET /metrics", promhttp.HandlerFor(a.registry, promhttp.HandlerOpts{}))
	return mux
}

// One bounded burst: 100 real HTTP requests, 10 workers, never recursive.
func (a *application) load(w http.ResponseWriter, r *http.Request) {
	select {
	case a.loadGate <- struct{}{}:
		defer func() { <-a.loadGate }()
	default:
		http.Error(w, "load already running", http.StatusTooManyRequests)
		return
	}
	var success atomic.Int64
	var wg sync.WaitGroup
	for worker := 0; worker < 10; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, a.selfURL+"/health", nil)
				if err != nil {
					continue
				}
				resp, err := a.client.Do(req)
				if err != nil {
					continue
				}
				_, readErr := io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode == 200 && readErr == nil {
					success.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	w.Header().Set("Content-Type", "application/json")
	if success.Load() != 100 {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(map[string]int64{"requested": 100, "succeeded": success.Load(), "failed": 100 - success.Load()})
}

func configureTracing(ctx context.Context) (*sdktrace.TracerProvider, error) {
	name := os.Getenv("OTEL_SERVICE_NAME")
	if name == "" {
		name = "api"
	}
	opts := []sdktrace.TracerProviderOption{sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", name)))}
	// Without an endpoint, spans still have valid IDs, but are not exported.
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" {
		exporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, err
		}
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}
	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	tp, err := configureTracing(ctx)
	if err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tp.Shutdown(c)
	}()
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	a := newApplication(logger)
	a.selfURL = fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)
	server := &http.Server{Handler: a.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	logger.Info("server_started", "address", listener.Addr().String(), "traces_export_enabled", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "")
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(c)
	}
}
func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).Error("server_failed", "error", err)
		os.Exit(1)
	}
}
