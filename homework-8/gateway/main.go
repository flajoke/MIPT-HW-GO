package main

import (
	"context"
	"errors"
	"homework-8/gateway/internal/api"
	"homework-8/ledger"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start, cancel := context.WithTimeout(ctx, 10*time.Second)
	service, closeFn, err := ledger.Open(start)
	cancel()
	if err != nil {
		return err
	}
	defer closeFn()
	server := &http.Server{Addr: ":8080", Handler: api.NewHandler(service), ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { log.Println("Gateway listening on http://localhost:8080"); done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			server.Close()
			return err
		}
		err := <-done
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
