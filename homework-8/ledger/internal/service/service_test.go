package service

import (
	"context"
	"errors"
	"fmt"
	"homework-8/ledger/internal/domain"
	"math"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"testing"
)

type memory struct {
	mu                                                               sync.Mutex
	budgets                                                          map[string]domain.Budget
	expenses                                                         []domain.Transaction
	getErr, sumErr, addErr, upsertErr, listErr, commitErr, reportErr error
	reportCalls                                                      int
}
type budgetRepo struct{ m *memory }
type expenseRepo struct{ m *memory }

func (b budgetRepo) Upsert(_ context.Context, value domain.Budget) error {
	if b.m.upsertErr != nil {
		return b.m.upsertErr
	}
	b.m.budgets[value.Category] = value
	return nil
}
func (b budgetRepo) GetByCategory(_ context.Context, c string) (domain.Budget, error) {
	if b.m.getErr != nil {
		return domain.Budget{}, b.m.getErr
	}
	value, ok := b.m.budgets[c]
	if !ok {
		return value, domain.ErrNotFound
	}
	return value, nil
}
func (b budgetRepo) List(context.Context) ([]domain.Budget, error) {
	b.m.mu.Lock()
	defer b.m.mu.Unlock()
	result := make([]domain.Budget, 0)
	for _, v := range b.m.budgets {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Category < result[j].Category })
	return result, b.m.listErr
}
func (e expenseRepo) Add(_ context.Context, v domain.Transaction) (domain.Transaction, error) {
	if e.m.addErr != nil {
		return domain.Transaction{}, e.m.addErr
	}
	v.ID = len(e.m.expenses) + 1
	e.m.expenses = append(e.m.expenses, v)
	return v, nil
}
func (e expenseRepo) SumByCategory(_ context.Context, c string) (float64, error) {
	var sum int64
	for _, v := range e.m.expenses {
		if v.Category == c {
			sum += int64(math.Round(v.Amount * 100))
		}
	}
	return float64(sum) / 100, e.m.sumErr
}
func (e expenseRepo) List(context.Context) ([]domain.Transaction, error) {
	e.m.mu.Lock()
	defer e.m.mu.Unlock()
	out := append(make([]domain.Transaction, 0), e.m.expenses...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].ID > out[j].ID
	})
	return out, e.m.listErr
}
func (m *memory) WithinCategory(ctx context.Context, c string, fn func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	before := make(map[string]domain.Budget)
	for k, v := range m.budgets {
		before[k] = v
	}
	txs := append([]domain.Transaction(nil), m.expenses...)
	err := fn(budgetRepo{m}, expenseRepo{m})
	if err == nil {
		err = m.commitErr
	}
	if err != nil {
		m.budgets = before
		m.expenses = txs
	}
	return err
}
func (m *memory) Summary(_ context.Context, from, to string) (domain.ReportSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reportCalls++
	if m.reportErr != nil {
		return domain.ReportSummary{}, m.reportErr
	}
	result := domain.ReportSummary{From: from, To: to, Categories: make([]domain.CategorySummary, 0)}
	sums := map[string]int64{}
	var total int64
	for _, v := range m.expenses {
		if v.Date >= from && v.Date <= to {
			amount := int64(math.Round(v.Amount * 100))
			sums[v.Category] += amount
			total += amount
		}
	}
	for c, amount := range sums {
		result.Categories = append(result.Categories, domain.CategorySummary{Category: c, Amount: float64(amount) / 100})
	}
	sort.Slice(result.Categories, func(i, j int) bool { return result.Categories[i].Category < result.Categories[j].Category })
	result.Total = float64(total) / 100
	return result, nil
}

type cacheStub struct {
	values  map[string][]byte
	version int
	broken  bool
}

func (c *cacheStub) Get(_ context.Context, key string) ([]byte, error) {
	if c.broken {
		return nil, errors.New("redis down")
	}
	value, ok := c.values[key]
	if !ok {
		return nil, errors.New("miss")
	}
	return value, nil
}
func (c *cacheStub) Set(_ context.Context, key string, value []byte) error {
	c.values[key] = value
	return nil
}
func (c *cacheStub) Version(context.Context) (string, error) {
	if c.broken {
		return "", errors.New("redis down")
	}
	return strconv.Itoa(c.version), nil
}
func (c *cacheStub) Invalidate(context.Context) error {
	if c.broken {
		return errors.New("redis down")
	}
	c.version++
	return nil
}
func fixture() (*Ledger, *memory) {
	m := &memory{budgets: map[string]domain.Budget{}}
	return New(budgetRepo{m}, expenseRepo{m}, m, m, nil), m
}
func tx(amount float64) domain.Transaction {
	return domain.Transaction{Amount: amount, Category: "food", Date: "2026-10-25", Description: "lunch"}
}
func set(t *testing.T, s *Ledger, limit float64) {
	t.Helper()
	if err := s.SetBudget(context.Background(), domain.Budget{Category: "food", Limit: limit}); err != nil {
		t.Fatal(err)
	}
}
func add(t *testing.T, s *Ledger, amount float64) domain.Transaction {
	t.Helper()
	saved, err := s.AddTransaction(context.Background(), tx(amount))
	if err != nil {
		t.Fatal(err)
	}
	return saved
}
func TestBudgetRules(t *testing.T) {
	for _, tt := range []struct {
		name        string
		first, next float64
		exceeded    bool
	}{{"within_limit", 1000, 2000, false}, {"exact_limit", 3000, 2000, false}, {"exceeded", 3000, 2001, true}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, _ := fixture()
			set(t, s, 5000)
			add(t, s, tt.first)
			before, _ := s.ListTransactions(context.Background())
			saved, err := s.AddTransaction(context.Background(), tx(tt.next))
			if errors.Is(err, domain.ErrBudgetExceeded) != tt.exceeded {
				t.Fatal(err)
			}
			after, _ := s.ListTransactions(context.Background())
			if tt.exceeded {
				if !reflect.DeepEqual(before, after) {
					t.Fatal("refused expense changed storage")
				}
			} else if err != nil || saved.ID != 2 || len(after) != 2 {
				t.Fatal(saved, err)
			}
		})
	}
	t.Run("no_budget", func(t *testing.T) { t.Parallel(); s, _ := fixture(); add(t, s, 10000) })
	t.Run("decimal_limit", func(t *testing.T) {
		t.Parallel()
		s, _ := fixture()
		set(t, s, .30)
		add(t, s, .10)
		add(t, s, .20)
		if _, err := s.AddTransaction(context.Background(), tx(.01)); !errors.Is(err, domain.ErrBudgetExceeded) {
			t.Fatal(err)
		}
	})
	t.Run("lowered_budget", func(t *testing.T) {
		t.Parallel()
		s, _ := fixture()
		set(t, s, 500)
		add(t, s, 450)
		set(t, s, 400)
		if _, err := s.AddTransaction(context.Background(), tx(1)); !errors.Is(err, domain.ErrBudgetExceeded) {
			t.Fatal(err)
		}
	})
}
func TestValidationAndUpdates(t *testing.T) {
	s, _ := fixture()
	ctx := context.Background()
	var validation *domain.ValidationError
	if _, err := s.AddTransaction(ctx, tx(0)); !errors.As(err, &validation) {
		t.Fatal("expected validation error", err)
	}
	if err := s.SetBudget(ctx, domain.Budget{Category: "food", Limit: 0}); !errors.As(err, &validation) {
		t.Fatal(err)
	}
	values, _ := s.ListTransactions(ctx)
	if len(values) != 0 {
		t.Fatal(values)
	}
	set(t, s, 500)
	set(t, s, 600)
	budgets, _ := s.ListBudgets(ctx)
	if len(budgets) != 1 || budgets[0].Limit != 600 {
		t.Fatal(budgets)
	}
}
func TestRepositoryErrors(t *testing.T) {
	sentinel := errors.New("storage unavailable")
	for _, name := range []string{"get", "sum", "add", "commit"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, m := fixture()
			set(t, s, 500)
			switch name {
			case "get":
				m.getErr = sentinel
			case "sum":
				m.sumErr = sentinel
			case "add":
				m.addErr = sentinel
			case "commit":
				m.commitErr = sentinel
			}
			if _, err := s.AddTransaction(context.Background(), tx(100)); !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
			if len(m.expenses) != 0 {
				t.Fatal("failed operation persisted")
			}
		})
	}
	t.Run("upsert", func(t *testing.T) {
		s, m := fixture()
		m.upsertErr = sentinel
		if err := s.SetBudget(context.Background(), domain.Budget{Category: "food", Limit: 500}); !errors.Is(err, sentinel) {
			t.Fatal(err)
		}
	})
	t.Run("lists", func(t *testing.T) {
		s, m := fixture()
		m.listErr = sentinel
		if _, err := s.ListBudgets(context.Background()); !errors.Is(err, sentinel) {
			t.Fatal(err)
		}
		if _, err := s.ListTransactions(context.Background()); !errors.Is(err, sentinel) {
			t.Fatal(err)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		s, _ := fixture()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := s.AddTransaction(ctx, tx(100)); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}
func TestConcurrentBudget(t *testing.T) {
	s, _ := fixture()
	set(t, s, 100)
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.AddTransaction(context.Background(), tx(20)); results <- err }()
	}
	wg.Wait()
	close(results)
	success, exceeded := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, domain.ErrBudgetExceeded) {
			exceeded++
		} else {
			t.Fatal(err)
		}
	}
	if success != 5 || exceeded != 5 {
		t.Fatal(fmt.Sprintf("success=%d exceeded=%d", success, exceeded))
	}
}
func TestReportCache(t *testing.T) {
	s, m := fixture()
	cache := &cacheStub{values: map[string][]byte{}}
	s.cache = cache
	ctx := context.Background()
	add(t, s, 100)
	first, err := s.GetReportSummary(ctx, "2026-10-01", "2026-10-31")
	if err != nil || first.Total != 100 {
		t.Fatal(first, err)
	}
	if _, err = s.GetReportSummary(ctx, "2026-10-01", "2026-10-31"); err != nil || m.reportCalls != 1 {
		t.Fatal("expected cache hit", err)
	}
	add(t, s, 25)
	result, err := s.GetReportSummary(ctx, "2026-10-01", "2026-10-31")
	if err != nil || result.Total != 125 || m.reportCalls != 2 {
		t.Fatal(result, err)
	}
	cache.broken = true
	result, err = s.GetReportSummary(ctx, "2026-10-01", "2026-10-31")
	if err != nil || result.Total != 125 {
		t.Fatal(result, err)
	}
	m.reportErr = errors.New("query failed")
	if _, err = s.GetReportSummary(ctx, "2026-10-01", "2026-10-31"); err == nil {
		t.Fatal("missing report error")
	}
	var validation *domain.ValidationError
	if _, err = s.GetReportSummary(ctx, "bad", "bad"); !errors.As(err, &validation) {
		t.Fatal(err)
	}
}
