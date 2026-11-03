package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"homework-8/ledger"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

type stubService struct {
	budget          ledger.Budget
	transaction     ledger.Transaction
	err             error
	seenBudget      ledger.Budget
	seenTransaction ledger.Transaction
	from, to        string
}

func (s *stubService) SetBudget(ctx context.Context, b ledger.Budget) error {
	s.seenBudget = b
	return s.err
}
func (s *stubService) ListBudgets(context.Context) ([]ledger.Budget, error) {
	if s.budget.Category == "" {
		return nil, s.err
	}
	return []ledger.Budget{s.budget}, s.err
}
func (s *stubService) AddTransaction(_ context.Context, tx ledger.Transaction) (ledger.Transaction, error) {
	s.seenTransaction = tx
	return s.transaction, s.err
}
func (s *stubService) ListTransactions(context.Context) ([]ledger.Transaction, error) {
	if s.transaction.ID == 0 {
		return nil, s.err
	}
	return []ledger.Transaction{s.transaction}, s.err
}
func (s *stubService) GetReportSummary(_ context.Context, from, to string) (ledger.ReportSummary, error) {
	s.from, s.to = from, to
	return ledger.ReportSummary{From: from, To: to, Total: 450, Categories: []ledger.CategorySummary{{Category: "food", Amount: 450}}}, s.err
}

var _ ledger.Service = (*stubService)(nil)

func call(t *testing.T, h http.Handler, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	if w.Code != status {
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, w.Code, status, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatal("missing JSON header")
	}
	if !json.Valid(w.Body.Bytes()) {
		t.Fatal("invalid JSON")
	}
	if status >= 400 {
		var v map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || len(v) != 1 || v["error"] == "" {
			t.Fatal("invalid error format")
		}
	}
	return w
}
func TestHTTPContracts(t *testing.T) {
	budget := ledger.Budget{Category: "food", Limit: 500}
	tx := ledger.Transaction{ID: 1, Amount: 450, Category: "food", Description: "lunch", Date: "2026-10-25"}
	s := &stubService{budget: budget, transaction: tx}
	h := NewHandler(s)
	t.Run("budget_post", func(t *testing.T) {
		w := call(t, h, "POST", "/api/budgets", `{"category":"food","limit":500}`, 201)
		var got BudgetResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got != budgetResponse(budget) || s.seenBudget != budget {
			t.Fatal(got, err)
		}
	})
	t.Run("budget_get", func(t *testing.T) {
		w := call(t, h, "GET", "/api/budgets", "", 200)
		var got []BudgetResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, []BudgetResponse{budgetResponse(budget)}) {
			t.Fatal(got, err)
		}
	})
	t.Run("transaction_post", func(t *testing.T) {
		w := call(t, h, "POST", "/api/transactions", `{"amount":450,"category":"food","description":"lunch","date":"2026-10-25"}`, 201)
		var got TransactionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got != transactionResponse(tx) {
			t.Fatal(got, err)
		}
		want := tx
		want.ID = 0
		if s.seenTransaction != want {
			t.Fatal(s.seenTransaction)
		}
	})
	t.Run("transaction_get", func(t *testing.T) {
		w := call(t, h, "GET", "/api/transactions", "", 200)
		var got []TransactionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, []TransactionResponse{transactionResponse(tx)}) {
			t.Fatal(got, err)
		}
	})
	t.Run("report", func(t *testing.T) {
		w := call(t, h, "GET", "/api/reports/summary?from=2026-10-01&to=2026-10-31", "", 200)
		var got ledger.ReportSummary
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Total != 450 || s.from != "2026-10-01" || s.to != "2026-10-31" {
			t.Fatal(got, err)
		}
	})
}
func TestHTTPErrorMapping(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{{"validation", &ledger.ValidationError{Err: errors.New("invalid input")}, 400}, {"exceeded", fmt.Errorf("wrapped: %w", ledger.ErrBudgetExceeded), 409}, {"internal", errors.New("private database details"), 500}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewHandler(&stubService{err: tt.err})
			w := call(t, h, "POST", "/api/transactions", `{"amount":450,"category":"food","date":"2026-10-25"}`, tt.status)
			if tt.status == 409 && strings.TrimSpace(w.Body.String()) != `{"error":"budget exceeded"}` {
				t.Fatal(w.Body.String())
			}
			if tt.status == 500 && strings.Contains(w.Body.String(), "private") {
				t.Fatal("storage details leaked")
			}
		})
	}
	for _, path := range []string{"/api/budgets", "/api/transactions", "/api/reports/summary?from=2026-10-01&to=2026-10-31"} {
		t.Run("query_error"+path, func(t *testing.T) {
			call(t, NewHandler(&stubService{err: errors.New("db down")}), "GET", path, "", 500)
		})
	}
	t.Run("budget_validation", func(t *testing.T) {
		call(t, NewHandler(&stubService{err: &ledger.ValidationError{Err: errors.New("invalid limit")}}), "POST", "/api/budgets", `{"category":"food","limit":0}`, 400)
	})
	t.Run("report_validation", func(t *testing.T) {
		call(t, NewHandler(&stubService{err: &ledger.ValidationError{Err: errors.New("invalid range")}}), "GET", "/api/reports/summary?from=bad&to=bad", "", 400)
	})
}
func TestEmptyAndInvalidHTTP(t *testing.T) {
	h := NewHandler(&stubService{})
	for _, path := range []string{"/api/budgets", "/api/transactions"} {
		t.Run("empty"+path, func(t *testing.T) {
			w := call(t, h, "GET", path, "", 200)
			if strings.TrimSpace(w.Body.String()) != "[]" {
				t.Fatal(w.Body.String())
			}
			call(t, h, "DELETE", path, "", 405)
		})
	}
	for _, body := range []string{"{", `[]`, `{} {}`, `{"amount":"wrong"}`, `{"unknown":1}`, ""} {
		t.Run("bad_json"+body, func(t *testing.T) { call(t, h, "POST", "/api/transactions", body, 400) })
	}
	call(t, h, "POST", "/api/budgets", "{", 400)
	call(t, h, "POST", "/api/reports/summary", "", 405)
	call(t, h, "GET", "/missing", "", 404)
}

func TestPostgresAPI(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a migrated *_test PostgreSQL database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("TEST_DATABASE_URL must be a PostgreSQL URL with a *_test database")
	}
	t.Setenv("DATABASE_URL", dsn)
	index := os.Getenv("TEST_REDIS_DB")
	if index == "" {
		index = "14"
	}
	t.Setenv("REDIS_DB", index)
	service, closeFn, err := ledger.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeFn != nil {
			closeFn()
		}
	}()
	h := NewHandler(service)
	category := fmt.Sprintf("http-test-%d", time.Now().UnixNano())
	budget := fmt.Sprintf(`{"category":%q,"limit":500}`, category)
	expense := fmt.Sprintf(`{"amount":450,"category":%q,"description":"lunch","date":"2026-10-25"}`, category)
	call(t, h, "POST", "/api/budgets", budget, 201)
	call(t, h, "POST", "/api/transactions", expense, 201)
	before, err := service.ListTransactions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	call(t, h, "POST", "/api/transactions", expense, 409)
	after, err := service.ListTransactions(context.Background())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("rejected expense changed storage", err)
	}
	path := "/api/reports/summary?from=2026-10-01&to=2026-10-31"
	call(t, h, "GET", path, "", 200)
	call(t, h, "GET", path, "", 200)
	if err := closeFn(); err != nil {
		t.Fatal(err)
	}
	closeFn = nil
	service, closeFn, err = ledger.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := service.ListTransactions(context.Background())
	if err != nil || !reflect.DeepEqual(before, persisted) {
		t.Fatal("restart lost data", err)
	}
	call(t, NewHandler(service), "GET", "/api/budgets", "", 200)
}
