// order-processor is a small stand-in for a real order-processing worker.
// It exposes liveness, readiness, and Prometheus metrics endpoints and runs a
// background processing loop. It is intentionally dependency-free.
//
// Read this file before you deploy it: the application already implements
// startup warmup, readiness gating, and graceful SIGTERM draining. Your job is
// to make the *deployment* reflect how the app actually behaves.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	ready     atomic.Bool
	processed atomic.Int64
	inflight  atomic.Int64
	startTime = time.Now()
)

// warmup simulates cache priming / dependency checks before the service is
// safe to receive traffic.
const warmup = 5 * time.Second

// drainTimeout is how long the service will spend finishing in-flight work
// after receiving SIGTERM before it exits.
const drainTimeout = 20 * time.Second

func main() {
	port := getenv("PORT", "8080")

	go func() {
		time.Sleep(warmup)
		ready.Store(true)
		log.Printf("ready after %s warmup", warmup)
	}()

	stop := make(chan struct{})
	go processLoop(stop)

	mux := http.NewServeMux()

	// Liveness: is the process itself healthy? Must NOT depend on downstream
	// dependencies, or a dependency blip will restart every pod at once.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Readiness: is it safe to send this pod traffic yet? Fails during warmup
	// and during shutdown so the pod is pulled from Service endpoints.
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "not ready")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ready")
	})

	mux.HandleFunc("/metrics", metricsHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "order-processor")
	})

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	go func() {
		log.Printf("listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutdown signal received, draining")

	ready.Store(false) // fail readiness -> removed from Service endpoints
	close(stop)

	ctx, cancel := context.WithTimeout(context.Background(), drainTimeout+5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)

	deadline := time.Now().Add(drainTimeout)
	for inflight.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	log.Printf("drained; processed=%d; exiting", processed.Load())
}

func processLoop(stop <-chan struct{}) {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			inflight.Add(1)
			time.Sleep(50 * time.Millisecond) // pretend to do work
			processed.Add(1)
			inflight.Add(-1)
		}
	}
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprint(w, "# HELP orders_processed_total Total orders processed.\n")
	fmt.Fprint(w, "# TYPE orders_processed_total counter\n")
	fmt.Fprintf(w, "orders_processed_total %d\n", processed.Load())
	fmt.Fprint(w, "# HELP orders_inflight Orders currently being processed.\n")
	fmt.Fprint(w, "# TYPE orders_inflight gauge\n")
	fmt.Fprintf(w, "orders_inflight %d\n", inflight.Load())
	fmt.Fprint(w, "# HELP process_uptime_seconds Process uptime in seconds.\n")
	fmt.Fprint(w, "# TYPE process_uptime_seconds gauge\n")
	fmt.Fprintf(w, "process_uptime_seconds %f\n", time.Since(startTime).Seconds())
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
