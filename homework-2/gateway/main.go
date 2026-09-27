package main

import (
	"fmt"
	"log"
	"net/http"
)

func pingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "pong")
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", pingHandler)

	fmt.Println("Gateway service starting: http://localhost:8080/ping")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
