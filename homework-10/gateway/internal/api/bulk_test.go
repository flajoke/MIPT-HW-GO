package api

import (
	"context"
	"encoding/json"
	"homework-10/ledger"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type bulkStub struct {
	stubService
	values  []ledger.Transaction
	workers int
	result  ledger.BulkResult
	wait    bool
	cancel  context.CancelFunc
}

func (s *bulkStub) ImportTransactions(ctx context.Context, values []ledger.Transaction, workers int) (ledger.BulkResult, error) {
	s.values = values
	s.workers = workers
	if s.wait {
		<-ctx.Done()
		return s.result, ctx.Err()
	}
	if s.cancel != nil {
		s.cancel()
	}
	return s.result, s.err
}
func bulkCall(t *testing.T, h http.Handler, path, body string, status int) map[string]json.RawMessage {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
	if w.Code != status || w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatal(w.Code, w.Body.String())
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestBulkHTTPMixedSuccess(t *testing.T) {
	s := &bulkStub{result: ledger.BulkResult{Accepted: 1, Rejected: 1, Errors: []ledger.BulkError{{Index: 1, Error: "budget exceeded"}}}}
	body := `[{"amount":100,"category":"food","description":"lunch","date":"2026-11-21"},{"amount":900,"category":"food","date":"2026-11-21"}]`
	result := bulkCall(t, NewHandler(s), "/api/transactions/bulk?workers=2", body, 200)
	if string(result["accepted"]) != "1" || string(result["rejected"]) != "1" || string(result["unprocessed"]) != "0" || s.workers != 2 || len(s.values) != 2 || s.values[0].Amount != 100 || s.values[0].Description != "lunch" {
		t.Fatal(result, s.workers, s.values)
	}
	var errs []BulkErrorResponse
	if err := json.Unmarshal(result["errors"], &errs); err != nil || len(errs) != 1 || errs[0].Index != 1 {
		t.Fatal(errs, err)
	}
}
func TestBulkHTTPEmptyAndDefault(t *testing.T) {
	s := &bulkStub{}
	result := bulkCall(t, NewHandler(s), "/api/transactions/bulk", "[]", 200)
	if string(result["errors"]) != "[]" || s.workers != ledger.DefaultBulkWorkers {
		t.Fatal(result, s.workers)
	}
}
func TestBulkHTTPInvalidRequests(t *testing.T) {
	for _, query := range []string{"?workers=0", "?workers=-1", "?workers=33", "?workers=no", "?workers=", "?workers=1&workers=2"} {
		call(t, NewHandler(&bulkStub{}), "POST", "/api/transactions/bulk"+query, "[]", 400)
	}
	for _, body := range []string{`null`, `{}`, `[`, `[] []`, `[{"amount":"wrong"}]`} {
		call(t, NewHandler(&bulkStub{}), "POST", "/api/transactions/bulk", body, 400)
	}
	call(t, NewHandler(&bulkStub{}), "GET", "/api/transactions/bulk", "", 405)
}
func TestBulkHTTPTimeoutKeepsSummary(t *testing.T) {
	s := &bulkStub{wait: true, result: ledger.BulkResult{Accepted: 2, Rejected: 1, Unprocessed: 3, Errors: []ledger.BulkError{{Index: 1, Error: "invalid amount"}}}}
	result := bulkCall(t, NewHandler(s, 10*time.Millisecond), "/api/transactions/bulk", "[]", 504)
	if string(result["error"]) != `"request timeout"` || string(result["accepted"]) != "2" || string(result["rejected"]) != "1" || string(result["unprocessed"]) != "3" {
		t.Fatal(result)
	}
}
func TestBulkHTTPContextCanceledAfterFinalWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &bulkStub{cancel: cancel, result: ledger.BulkResult{Accepted: 1}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/transactions/bulk", strings.NewReader(`[{}]`)).WithContext(ctx)
	NewHandler(s).ServeHTTP(w, r)
	var response struct {
		Error    string `json:"error"`
		Accepted int    `json:"accepted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 504 || response.Accepted != 1 || response.Error != "request timeout" {
		t.Fatal(w.Code, w.Body.String(), err)
	}
}
