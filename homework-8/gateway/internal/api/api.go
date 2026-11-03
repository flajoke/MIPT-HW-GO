package api

import (
	"encoding/json"
	"errors"
	"homework-8/ledger"
	"io"
	"log"
	"net/http"
	"time"
)

type Handler struct{ service ledger.Service }

func NewHandler(service ledger.Service) http.Handler {
	h := &Handler{service: service}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/transactions", h.transactions)
	mux.HandleFunc("/api/budgets", h.budgets)
	mux.HandleFunc("/api/reports/summary", h.reportSummary)
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
func (h *Handler) transactions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result := make([]TransactionResponse, 0)
		saved, err := h.service.ListTransactions(r.Context())
		if err != nil {
			log.Printf("list transactions: %v", err)
			writeError(w, 500, "internal server error")
			return
		}
		for _, tx := range saved {
			result = append(result, transactionResponse(tx))
		}
		writeJSON(w, 200, result)
	case http.MethodPost:
		var req CreateTransactionRequest
		if !decode(w, r, &req) {
			return
		}
		tx := req.toDomain()

		// AddTransaction returns the saved transaction with its assigned ID.
		saved, err := h.service.AddTransaction(r.Context(), tx)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, 201, transactionResponse(saved))
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, 405, "method not allowed")
	}
}
func (h *Handler) budgets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result := make([]BudgetResponse, 0)
		saved, err := h.service.ListBudgets(r.Context())
		if err != nil {
			log.Printf("list budgets: %v", err)
			writeError(w, 500, "internal server error")
			return
		}
		for _, b := range saved {
			result = append(result, budgetResponse(b))
		}
		writeJSON(w, 200, result)
	case http.MethodPost:
		var req CreateBudgetRequest
		if !decode(w, r, &req) {
			return
		}
		b := req.toDomain()

		if err := h.service.SetBudget(r.Context(), b); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, 201, budgetResponse(b))
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, 405, "method not allowed")
	}
}

func (h *Handler) reportSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeError(w, 405, "method not allowed")
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")

	report, err := h.service.GetReportSummary(r.Context(), from, to)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, report)
}

func writeServiceError(w http.ResponseWriter, err error) {
	var validation *ledger.ValidationError
	switch {
	case errors.As(err, &validation):
		writeError(w, 400, validation.Error())
	case errors.Is(err, ledger.ErrBudgetExceeded):
		writeError(w, 409, "budget exceeded")
	default:
		log.Printf("service: %v", err)
		writeError(w, 500, "internal server error")
	}
}
