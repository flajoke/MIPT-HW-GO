package api

import (
	"context"
	"errors"
	"homework-10/ledger"
	"net/http"
	"strconv"
)

type BulkErrorResponse struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}
type BulkResponse struct {
	Accepted    int                 `json:"accepted"`
	Rejected    int                 `json:"rejected"`
	Errors      []BulkErrorResponse `json:"errors"`
	Unprocessed int                 `json:"unprocessed"`
}
type bulkTimeoutResponse struct {
	Error string `json:"error"`
	BulkResponse
}

func bulkResponse(value ledger.BulkResult) BulkResponse {
	result := BulkResponse{Accepted: value.Accepted, Rejected: value.Rejected, Unprocessed: value.Unprocessed, Errors: make([]BulkErrorResponse, 0, len(value.Errors))}
	for _, item := range value.Errors {
		result.Errors = append(result.Errors, BulkErrorResponse{Index: item.Index, Error: item.Error})
	}
	return result
}
func (h *Handler) bulkTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, 405, "method not allowed")
		return
	}
	workers := ledger.DefaultBulkWorkers
	if raw, exists := r.URL.Query()["workers"]; exists {
		var err error
		if len(raw) != 1 {
			writeError(w, 400, "workers must be one integer from 1 to 32")
			return
		}
		workers, err = strconv.Atoi(raw[0])
		if err != nil || workers < 1 || workers > ledger.MaxBulkWorkers {
			writeError(w, 400, "workers must be an integer from 1 to 32")
			return
		}
	}
	var request []CreateTransactionRequest
	if !decode(w, r, &request) {
		return
	}
	if request == nil {
		writeError(w, 400, "expected a JSON array, not null")
		return
	}
	transactions := make([]ledger.Transaction, len(request))
	for index, item := range request {
		transactions[index] = item.toDomain()
	}
	result, err := h.service.ImportTransactions(r.Context(), transactions, workers)
	response := bulkResponse(result)
	timedOut := bulkTimeoutResponse{Error: "request timeout", BulkResponse: response}
	// Preserve final accounting if the middleware observes cancellation after
	// the service finishes, including a successful final write at the deadline.
	if writer, ok := w.(interface{ SetTimeoutResponse(any) }); ok {
		writer.SetTimeoutResponse(timedOut)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			writeJSON(w, 504, timedOut)
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, response)
}
