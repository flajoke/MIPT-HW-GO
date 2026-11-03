package domain

import "context"

type BudgetRepository interface {
	Upsert(context.Context, Budget) error
	GetByCategory(context.Context, string) (Budget, error)
	List(context.Context) ([]Budget, error)
}
type ExpenseRepository interface {
	Add(context.Context, Transaction) (Transaction, error)
	List(context.Context) ([]Transaction, error)
	SumByCategory(context.Context, string) (float64, error)
}
type ReportRepository interface {
	Summary(ctx context.Context, from, to string) (ReportSummary, error)
}

// UnitOfWork supplies repositories bound to one atomic category operation.
// The implementation decides how locking, commit and rollback are performed.
type UnitOfWork interface {
	WithinCategory(ctx context.Context, category string, fn func(BudgetRepository, ExpenseRepository) error) error
}
type ReportCache interface {
	Get(context.Context, string) ([]byte, error)
	Set(context.Context, string, []byte) error
	Version(context.Context) (string, error)
	Invalidate(context.Context) error
}
