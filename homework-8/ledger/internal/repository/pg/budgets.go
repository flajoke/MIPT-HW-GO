package pg

import (
	"context"
	"database/sql"
	"errors"
	"homework-8/ledger/internal/domain"
)

type queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type Budgets struct{ db queryer }

func NewBudgets(db queryer) *Budgets { return &Budgets{db: db} }

var _ domain.BudgetRepository = (*Budgets)(nil)

func (r *Budgets) Upsert(ctx context.Context, b domain.Budget) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO budgets(category,limit_amount) VALUES($1,$2)
 ON CONFLICT(category) DO UPDATE SET limit_amount=EXCLUDED.limit_amount`, b.Category, b.Limit)
	return err
}
func (r *Budgets) GetByCategory(ctx context.Context, category string) (domain.Budget, error) {
	var b domain.Budget
	err := r.db.QueryRowContext(ctx, "SELECT category,limit_amount FROM budgets WHERE category=$1", category).Scan(&b.Category, &b.Limit)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Budget{}, domain.ErrNotFound
	}
	return b, err
}
func (r *Budgets) List(ctx context.Context) ([]domain.Budget, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT category,limit_amount FROM budgets ORDER BY category")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Budget, 0)
	for rows.Next() {
		var b domain.Budget
		if err := rows.Scan(&b.Category, &b.Limit); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}
