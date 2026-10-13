package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL())
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer db.Close()

	processedOrders := prometheus.NewCounter(prometheus.CounterOpts{Name: "shop_worker_processed_orders_total", Help: "Orders successfully committed as processed by this worker."})
	workerErrors := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "shop_worker_errors_total", Help: "Worker database errors by operation."}, []string{"operation"})
	prometheus.MustRegister(processedOrders, workerErrors)
	workerErrors.WithLabelValues("schema").Add(0)
	workerErrors.WithLabelValues("process").Add(0)
	health := http.NewServeMux()
	health.Handle("GET /metrics", promhttp.Handler())
	health.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	go func() {
		addr := envOr("WORKER_ADDR", ":8081")
		log.Printf("worker health endpoint listening on %s", addr)
		if err := http.ListenAndServe(addr, health); err != nil {
			log.Fatalf("worker health server: %v", err)
		}
	}()

	interval := 2 * time.Second
	if raw := os.Getenv("POLL_INTERVAL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			log.Fatalf("POLL_INTERVAL must be a positive duration: %q", raw)
		}
		interval = parsed
	}

	log.Printf("worker polling every %s", interval)
	schemaReady := false
	for {
		if !schemaReady {
			if err := ensureSchema(ctx, db); err != nil {
				workerErrors.WithLabelValues("schema").Inc()
				log.Printf("initialize schema (will retry): %v", err)
				time.Sleep(interval)
				continue
			}
			schemaReady = true
		}
		processed, err := processOne(ctx, db)
		if err != nil {
			workerErrors.WithLabelValues("process").Inc()
			log.Printf("process order: %v", err)
			time.Sleep(interval)
			continue
		}
		if processed {
			processedOrders.Inc()
			continue
		}
		time.Sleep(interval)
	}
}

func ensureSchema(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS orders (
		id BIGSERIAL PRIMARY KEY,
		item TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		processed BOOLEAN NOT NULL DEFAULT false
	)`)
	return err
}

func processOne(ctx context.Context, db *pgxpool.Pool) (bool, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	err = tx.QueryRow(ctx, `SELECT id FROM orders WHERE processed = false ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE orders SET processed = true WHERE id = $1`, id); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	log.Printf("marked order %d processed", id)
	return true, nil
}

func databaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	user := envOr("POSTGRES_USER", "shop")
	password := envOr("POSTGRES_PASSWORD", "shop")
	database := envOr("POSTGRES_DB", "shop")
	host := envOr("POSTGRES_HOST", "postgres")
	port := envOr("POSTGRES_PORT", "5432")
	return "postgres://" + user + ":" + password + "@" + host + ":" + port + "/" + database + "?sslmode=disable"
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
