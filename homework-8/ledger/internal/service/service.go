package service

import (
	"context"
	"errors"
	"homework-8/ledger/internal/domain"
	"log"
	"math"
	"time"
)

type Service interface {
	SetBudget(context.Context, domain.Budget) error
	ListBudgets(context.Context) ([]domain.Budget, error)
	AddTransaction(context.Context, domain.Transaction) (domain.Transaction, error)
	ListTransactions(context.Context) ([]domain.Transaction, error)
	GetReportSummary(ctx context.Context, from, to string) (domain.ReportSummary, error)
}
type Ledger struct {
	budgets  domain.BudgetRepository
	expenses domain.ExpenseRepository
	reports  domain.ReportRepository
	unit     domain.UnitOfWork
	cache    domain.ReportCache
}

func New(b domain.BudgetRepository, e domain.ExpenseRepository, r domain.ReportRepository, u domain.UnitOfWork, c domain.ReportCache) *Ledger {
	return &Ledger{budgets: b, expenses: e, reports: r, unit: u, cache: c}
}

var _ Service = (*Ledger)(nil)

func bounded(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 5*time.Second)
}
func (s *Ledger) SetBudget(ctx context.Context, b domain.Budget) error {
	if err := b.Validate(); err != nil {
		return &domain.ValidationError{Err: err}
	}
	ctx, cancel := bounded(ctx)
	defer cancel()
	return s.unit.WithinCategory(ctx, b.Category, func(budgets domain.BudgetRepository, _ domain.ExpenseRepository) error { return budgets.Upsert(ctx, b) })
}
func (s *Ledger) ListBudgets(ctx context.Context) ([]domain.Budget, error) {
	ctx, cancel := bounded(ctx)
	defer cancel()
	return s.budgets.List(ctx)
}
func (s *Ledger) ListTransactions(ctx context.Context) ([]domain.Transaction, error) {
	ctx, cancel := bounded(ctx)
	defer cancel()
	return s.expenses.List(ctx)
}

// Inputs have cent precision. Converting to integer cents avoids 0.1+0.2 errors.
func cents(value float64) int64 { return int64(math.Round(value * 100)) }
func (s *Ledger) AddTransaction(ctx context.Context, value domain.Transaction) (domain.Transaction, error) {
	if err := value.Validate(); err != nil {
		return domain.Transaction{}, &domain.ValidationError{Err: err}
	}
	ctx, cancel := bounded(ctx)
	defer cancel()
	var saved domain.Transaction
	err := s.unit.WithinCategory(ctx, value.Category, func(budgets domain.BudgetRepository, expenses domain.ExpenseRepository) error {
		budget, err := budgets.GetByCategory(ctx, value.Category)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if err == nil {
			spent, err := expenses.SumByCategory(ctx, value.Category)
			if err != nil {
				return err
			}
			if spent > budget.Limit || cents(spent)+cents(value.Amount) > cents(budget.Limit) {
				return domain.ErrBudgetExceeded
			}
		}
		saved, err = expenses.Add(ctx, value)
		return err
	})
	if err != nil {
		return domain.Transaction{}, err
	}
	if s.cache != nil {
		if err := s.cache.Invalidate(ctx); err != nil {
			log.Printf("Redis: не удалось обновить версию отчётов: %v", err)
		}
	}
	return saved, nil
}
