package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type order struct {
	ID        int64     `json:"id"`
	Item      string    `json:"item"`
	CreatedAt time.Time `json:"created_at"`
	Processed bool      `json:"processed"`
}

func main() {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL())
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer db.Close()

	go func() {
		for {
			if err := ensureSchema(ctx, db); err == nil {
				log.Printf("database schema is ready")
				return
			} else {
				log.Printf("initialize schema (will retry): %v", err)
			}
			time.Sleep(3 * time.Second)
		}
	}()

	metrics := newAPIMetrics()
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(metrics.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		if strings.EqualFold(os.Getenv("HEALTH_FAIL"), "true") {
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /order", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Item string `json:"item"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil || strings.TrimSpace(input.Item) == "" {
			http.Error(w, "body must contain a non-empty item", http.StatusBadRequest)
			return
		}
		var created order
		err := db.QueryRow(r.Context(), `INSERT INTO orders(item) VALUES ($1) RETURNING id, item, created_at, processed`, strings.TrimSpace(input.Item)).Scan(&created.ID, &created.Item, &created.CreatedAt, &created.Processed)
		if err != nil {
			http.Error(w, "could not create order", http.StatusInternalServerError)
			log.Printf("create order: %v", err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	})
	mux.HandleFunc("GET /orders", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(r.Context(), `SELECT id, item, created_at, processed FROM orders ORDER BY id`)
		if err != nil {
			http.Error(w, "could not read orders", http.StatusInternalServerError)
			log.Printf("list orders: %v", err)
			return
		}
		defer rows.Close()
		orders := make([]order, 0)
		for rows.Next() {
			var o order
			if err := rows.Scan(&o.ID, &o.Item, &o.CreatedAt, &o.Processed); err != nil {
				http.Error(w, "could not read orders", http.StatusInternalServerError)
				return
			}
			orders = append(orders, o)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "could not read orders", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, orders)
	})

	addr := envOr("API_ADDR", ":8080")
	server := &http.Server{Addr: addr, Handler: metrics.handler(mux), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("api listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("http server: %v", err)
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

func databaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://shop:shop@postgres:5432/shop?sslmode=disable"
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
