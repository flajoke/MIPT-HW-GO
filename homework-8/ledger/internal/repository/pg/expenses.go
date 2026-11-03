package pg

import (
	"context"
	"homework-8/ledger/internal/domain"
	"time"
)

type Expenses struct{ db queryer }

func NewExpenses(db queryer) *Expenses { return &Expenses{db: db} }

var _ domain.ExpenseRepository = (*Expenses)(nil)

func (r *Expenses) Add(ctx context.Context, value domain.Transaction) (domain.Transaction, error) {
	err := r.db.QueryRowContext(ctx, `INSERT INTO expenses(amount,category,description,date)
 VALUES($1,$2,$3,$4) RETURNING id`, value.Amount, value.Category, value.Description, value.Date).Scan(&value.ID)
	if err != nil {
		return domain.Transaction{}, err
	}
	return value, nil
}
func (r *Expenses) List(ctx context.Context) ([]domain.Transaction, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,amount,category,COALESCE(description,''),date FROM expenses ORDER BY date DESC,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Transaction, 0)
	for rows.Next() {
		var value domain.Transaction
		var date time.Time
		if err := rows.Scan(&value.ID, &value.Amount, &value.Category, &value.Description, &date); err != nil {
			return nil, err
		}
		value.Date = date.Format("2006-01-02")
		result = append(result, value)
	}
	return result, rows.Err()
}
func (r *Expenses) SumByCategory(ctx context.Context, category string) (float64, error) {
	var sum float64
	err := r.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount),0) FROM expenses WHERE category=$1", category).Scan(&sum)
	return sum, err
}
