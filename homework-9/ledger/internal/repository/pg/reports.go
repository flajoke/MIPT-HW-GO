package pg

import (
	"context"
	"homework-9/ledger/internal/domain"
)

type Reports struct{ db queryer }

func NewReports(db queryer) *Reports { return &Reports{db: db} }

var _ domain.ReportRepository = (*Reports)(nil)

func (r *Reports) ListCategories(ctx context.Context, from, to string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT category FROM expenses
 WHERE date BETWEEN $1::date AND $2::date ORDER BY category`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	categories := make([]string, 0)
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}
func (r *Reports) SumForPeriod(ctx context.Context, category, from, to string) (float64, error) {
	var amount float64
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount),0) FROM expenses
 WHERE category=$1 AND date BETWEEN $2::date AND $3::date`, category, from, to).Scan(&amount)
	return amount, err
}
