package api

import (
	"encoding/json"
	"errors"
	"homework-5/ledger"
	"io"
	"log"
	"net/http"
	"time"
)

func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/transactions", transactions)
	mux.HandleFunc("/api/budgets", budgets)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { writeError(w, 404, "not found") })
	return LoggingMiddleware(jsonMiddleware(mux))
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() { log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start)) }()
		next.ServeHTTP(w, r)
	})
}
func jsonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{message})
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "invalid JSON: "+err.Error())
		return false
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		writeError(w, 400, "request must contain one JSON object")
		return false
	}
	return true
}
func transactions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result := make([]TransactionResponse, 0)
		for _, tx := range ledger.ListTransactions() {
			result = append(result, transactionResponse(tx))
		}
		writeJSON(w, 200, result)
	case http.MethodPost:
		var req CreateTransactionRequest
		if !decode(w, r, &req) {
			return
		}
		tx := req.toDomain()
		if err := tx.Validate(); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		// AddTransaction returns the saved transaction with its assigned ID.
		saved, err := ledger.AddTransaction(tx)
		if errors.Is(err, ledger.ErrBudgetExceeded) {
			writeError(w, 409, "budget exceeded")
			return
		}
		if err != nil {
			log.Printf("add transaction: %v", err)
			writeError(w, 500, "internal server error")
			return
		}
		writeJSON(w, 201, transactionResponse(saved))
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, 405, "method not allowed")
	}
}
func budgets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result := make([]BudgetResponse, 0)
		for _, b := range ledger.ListBudgets() {
			result = append(result, budgetResponse(b))
		}
		writeJSON(w, 200, result)
	case http.MethodPost:
		var req CreateBudgetRequest
		if !decode(w, r, &req) {
			return
		}
		b := req.toDomain()
		if err := b.Validate(); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		if err := ledger.SetBudget(b); err != nil {
			log.Printf("set budget: %v", err)
			writeError(w, 500, "internal server error")
			return
		}
		writeJSON(w, 201, budgetResponse(b))
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, 405, "method not allowed")
	}
}
