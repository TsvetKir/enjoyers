package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
)

// Ссылки на выделенные блоки сохраняются до завершения процесса.
var (
	memoryMu sync.Mutex
	blocks   [][]byte
	burnOnce sync.Once
)

func health(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok")
}

func eat(w http.ResponseWriter, r *http.Request) {
	mb, err := strconv.Atoi(r.URL.Query().Get("mb"))
	maxInt := int(^uint(0) >> 1)
	if err != nil || mb <= 0 || mb > maxInt/(1024*1024) {
		http.Error(w, "mb must be a positive integer that fits in memory addressing", http.StatusBadRequest)
		return
	}

	block := make([]byte, mb*1024*1024)
	// Записываем во весь блок, чтобы ОС действительно предоставила страницы памяти.
	for i := range block {
		block[i] = 1
	}

	// HTTP-запросы могут выполняться одновременно: защищаем общий список.
	memoryMu.Lock()
	blocks = append(blocks, block)
	memoryMu.Unlock()
	fmt.Fprintf(w, "holding another %d MiB\n", mb)
}

func burn(w http.ResponseWriter, r *http.Request) {
	// Только один вычислительный цикл, даже при повторных запросах.
	burnOnce.Do(func() {
		go func() {
			for {
			}
		}()
	})
	fmt.Fprintln(w, "CPU load is active until the process exits")
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /eat", eat)
	mux.HandleFunc("GET /burn", burn)

	// Обрабатываем завершение явно — пригодится при запуске в роли PID 1.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		os.Exit(0)
	}()

	log.Printf("api: pid=%d, listening on :8080", os.Getpid())
	log.Fatal(http.ListenAndServe(":8080", mux))
}
