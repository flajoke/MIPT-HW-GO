package main

import (
	"homework-6/gateway/internal/api"
	"log"
	"net/http"
	"time"
)

func main() {
	server := &http.Server{Addr: ":8080", Handler: api.NewHandler(), ReadHeaderTimeout: 5 * time.Second}
	log.Println("Gateway listening on http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}
