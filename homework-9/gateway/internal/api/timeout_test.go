package api

import (
	"context"
	"errors"
	"homework-9/ledger"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type blockingService struct {
	stubService
	active       atomic.Int32
	seenDeadline atomic.Bool
}

func (s *blockingService) block(ctx context.Context) error {
	s.active.Add(1)
	defer s.active.Add(-1)
	_, ok := ctx.Deadline()
	s.seenDeadline.Store(ok)
	<-ctx.Done()
	return ctx.Err()
}
func (s *blockingService) ListBudgets(ctx context.Context) ([]ledger.Budget, error) {
	return nil, s.block(ctx)
}
func (s *blockingService) ListTransactions(ctx context.Context) ([]ledger.Transaction, error) {
	return nil, s.block(ctx)
}
func (s *blockingService) SetBudget(ctx context.Context, b ledger.Budget) error { return s.block(ctx) }
func (s *blockingService) AddTransaction(ctx context.Context, tx ledger.Transaction) (ledger.Transaction, error) {
	return ledger.Transaction{}, s.block(ctx)
}
func (s *blockingService) GetReportSummary(ctx context.Context, from, to string) (ledger.ReportSummary, error) {
	return ledger.ReportSummary{}, s.block(ctx)
}
func TestTimeoutCoversAllEndpoints(t *testing.T) {
	for _, tt := range []struct{ method, path, body string }{{"GET", "/api/budgets", ""}, {"POST", "/api/budgets", `{"category":"food","limit":500}`}, {"GET", "/api/transactions", ""}, {"POST", "/api/transactions", `{"amount":1,"category":"food","date":"2026-11-21"}`}, {"GET", "/api/reports/summary?from=2026-11-01&to=2026-11-30", ""}} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			t.Parallel()
			s := &blockingService{}
			w := call(t, NewHandler(s, 10*time.Millisecond), tt.method, tt.path, tt.body, 504)
			if strings.TrimSpace(w.Body.String()) != `{"error":"request timeout"}` {
				t.Fatal(w.Body.String())
			}
			if s.active.Load() != 0 || !s.seenDeadline.Load() {
				t.Fatal("context not propagated or service left running")
			}
		})
	}
}
func TestTimeoutDiscardsLateResponse(t *testing.T) {
	h := TimeoutMiddleware(10 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		w.Write([]byte(`{"partial":true}`))
		<-r.Context().Done()
		w.Write([]byte(`{"late":true}`))
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/test", nil))
	if w.Code != 504 || strings.TrimSpace(w.Body.String()) != `{"error":"request timeout"}` {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestParentCancellation(t *testing.T) {
	s := &blockingService{}
	h := NewHandler(s)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/budgets", nil).WithContext(ctx))
	if w.Code != 504 || s.active.Load() != 0 {
		t.Fatal(w.Code)
	}
}
func TestWrappedContextErrors(t *testing.T) {
	for _, err := range []error{context.Canceled, context.DeadlineExceeded} {
		w := httptest.NewRecorder()
		writeServiceError(w, errors.Join(errors.New("operation stopped"), err))
		if w.Code != 504 {
			t.Fatal(w.Code)
		}
	}
}
