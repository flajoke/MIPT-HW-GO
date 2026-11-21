package service

import (
	"context"
	"errors"
	"homework-10/ledger/internal/domain"
	"sync/atomic"
	"testing"
	"time"
)

func TestBulkMixedResults(t *testing.T) {
	for _, workers := range []int{1, 4, 32} {
		t.Run(string(rune('A'+workers)), func(t *testing.T) {
			s, _ := fixture()
			set(t, s, 100)
			other := tx(30)
			other.Category = "other"
			values := []domain.Transaction{tx(60), tx(60), tx(0), other}
			result, err := s.ImportTransactions(context.Background(), values, workers)
			if err != nil || result.Accepted != 2 || result.Rejected != 2 || result.Unprocessed != 0 || len(result.Errors) != 2 {
				t.Fatal(result, err)
			}
			budgetError, validationError := false, false
			for _, item := range result.Errors {
				if item.Error == "budget exceeded" && (item.Index == 0 || item.Index == 1) {
					budgetError = true
				}
				if item.Index == 2 && item.Error != "" {
					validationError = true
				}
			}
			if !budgetError || !validationError || result.Errors[0].Index >= result.Errors[1].Index {
				t.Fatal(result.Errors)
			}
			saved, err := s.ListTransactions(context.Background())
			if err != nil || len(saved) != 2 {
				t.Fatal(saved, err)
			}
		})
	}
}
func TestBulkBudgetUnderContention(t *testing.T) {
	s, _ := fixture()
	set(t, s, 100)
	values := make([]domain.Transaction, 100)
	for i := range values {
		values[i] = tx(10)
	}
	result, err := s.ImportTransactions(context.Background(), values, 32)
	if err != nil || result.Accepted != 10 || result.Rejected != 90 || result.Unprocessed != 0 {
		t.Fatal(result, err)
	}
	saved, _ := s.ListTransactions(context.Background())
	if len(saved) != 10 {
		t.Fatal("budget overspent", len(saved))
	}
}
func TestBulkEmptyAndWorkerValidation(t *testing.T) {
	s, _ := fixture()
	empty, err := s.ImportTransactions(context.Background(), nil, 4)
	if err != nil || empty.Accepted != 0 || empty.Rejected != 0 || empty.Errors == nil || empty.Unprocessed != 0 {
		t.Fatal(empty, err)
	}
	for _, workers := range []int{-1, 0, 33} {
		var validation *domain.ValidationError
		if _, err := s.ImportTransactions(context.Background(), []domain.Transaction{tx(1)}, workers); !errors.As(err, &validation) {
			t.Fatal(workers, err)
		}
	}
}

type gatedUnit struct {
	base    domain.UnitOfWork
	started chan struct{}
	release chan struct{}
	active  atomic.Int32
	peak    atomic.Int32
}

func (g *gatedUnit) WithinCategory(ctx context.Context, c string, fn func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
	active := g.active.Add(1)
	defer g.active.Add(-1)
	for previous := g.peak.Load(); active > previous; previous = g.peak.Load() {
		if g.peak.CompareAndSwap(previous, active) {
			break
		}
	}
	g.started <- struct{}{}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.release:
	}
	return g.base.WithinCategory(ctx, c, fn)
}

type bulkOutcome struct {
	result domain.BulkResult
	err    error
}

func runBulk(s *Ledger, ctx context.Context, values []domain.Transaction, n int) <-chan bulkOutcome {
	done := make(chan bulkOutcome, 1)
	go func() { r, err := s.ImportTransactions(ctx, values, n); done <- bulkOutcome{r, err} }()
	return done
}
func awaitBulk(t *testing.T, done <-chan bulkOutcome) bulkOutcome {
	t.Helper()
	select {
	case value := <-done:
		return value
	case <-time.After(time.Second):
		t.Fatal("bulk import did not terminate")
		return bulkOutcome{}
	}
}
func TestBulkPoolSizeAndParallelStart(t *testing.T) {
	s, m := fixture()
	g := &gatedUnit{base: m, started: make(chan struct{}, 5), release: make(chan struct{})}
	s.unit = g
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	values := []domain.Transaction{tx(1), tx(1), tx(1), tx(1), tx(1)}
	done := runBulk(s, ctx, values, 2)
	for i := 0; i < 2; i++ {
		select {
		case <-g.started:
		case <-time.After(time.Second):
			cancel()
			awaitBulk(t, done)
			t.Fatal("workers did not start concurrently")
		}
	}
	if g.active.Load() != 2 {
		cancel()
		awaitBulk(t, done)
		t.Fatal("wrong pool size")
	}
	close(g.release)
	result := awaitBulk(t, done)
	if result.err != nil || result.result.Accepted != 5 || g.peak.Load() != 2 || g.active.Load() != 0 {
		t.Fatal(result, g.peak.Load(), g.active.Load())
	}
}
func TestBulkCancelStopsWorkers(t *testing.T) {
	s, m := fixture()
	g := &gatedUnit{base: m, started: make(chan struct{}, 4), release: make(chan struct{})}
	s.unit = g
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	values := make([]domain.Transaction, 100)
	for i := range values {
		values[i] = tx(1)
	}
	done := runBulk(s, ctx, values, 4)
	for i := 0; i < 4; i++ {
		select {
		case <-g.started:
		case <-time.After(time.Second):
			cancel()
			awaitBulk(t, done)
			t.Fatal("workers did not start")
		}
	}
	cancel()
	result := awaitBulk(t, done)
	if !errors.Is(result.err, context.Canceled) || result.result.Unprocessed != 100 || result.result.Accepted != 0 || result.result.Rejected != 0 || g.active.Load() != 0 {
		t.Fatal(result, g.active.Load())
	}
}

type cancelAfterCommit struct {
	base   domain.UnitOfWork
	cancel context.CancelFunc
}

func (c cancelAfterCommit) WithinCategory(ctx context.Context, category string, fn func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
	err := c.base.WithinCategory(ctx, category, fn)
	if err == nil {
		c.cancel()
	}
	return err
}
func TestBulkKeepsCommittedOutcomeAfterCancel(t *testing.T) {
	s, m := fixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.unit = cancelAfterCommit{base: m, cancel: cancel}
	values := []domain.Transaction{tx(10), tx(10), tx(10)}
	result, err := s.ImportTransactions(ctx, values, 1)
	if !errors.Is(err, context.Canceled) || result.Accepted != 1 || result.Rejected != 0 || result.Unprocessed != 2 {
		t.Fatal(result, err)
	}
	saved, _ := s.ListTransactions(context.Background())
	if len(saved) != result.Accepted {
		t.Fatal("a committed outcome was dropped", saved, result)
	}
}
func TestBulkDelayDeadline(t *testing.T) {
	s, _ := fixture()
	s.options.BulkWorkDelay = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	outcome := awaitBulk(t, runBulk(s, ctx, []domain.Transaction{tx(1), tx(1)}, 2))
	if !errors.Is(outcome.err, context.DeadlineExceeded) || outcome.result.Unprocessed != 2 {
		t.Fatal(outcome)
	}
}
func TestBulkAlreadyCanceled(t *testing.T) {
	s, _ := fixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := s.ImportTransactions(ctx, []domain.Transaction{tx(1)}, 4)
	if !errors.Is(err, context.Canceled) || result.Unprocessed != 1 || result.Errors == nil {
		t.Fatal(result, err)
	}
}
func TestBulkStorageErrorIsSanitized(t *testing.T) {
	s, m := fixture()
	m.addErr = errors.New("secret SQL details")
	result, err := s.ImportTransactions(context.Background(), []domain.Transaction{tx(1)}, 1)
	if err != nil || result.Rejected != 1 || result.Errors[0].Error != "internal server error" {
		t.Fatal(result, err)
	}
}
