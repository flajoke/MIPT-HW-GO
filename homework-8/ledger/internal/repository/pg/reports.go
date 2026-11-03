package pg

import (
	"context"
	"fmt"
	"homework-8/ledger/internal/domain"
	"math/big"
)

type Reports struct{ db queryer }

func NewReports(db queryer) *Reports { return &Reports{db: db} }

var _ domain.ReportRepository = (*Reports)(nil)

func (r *Reports) Summary(ctx context.Context, from, to string) (domain.ReportSummary, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT category,SUM(amount)::text FROM expenses
 WHERE date BETWEEN $1::date AND $2::date GROUP BY category ORDER BY category`, from, to)
	if err != nil {
		return domain.ReportSummary{}, err
	}
	defer rows.Close()
	result := domain.ReportSummary{From: from, To: to, Categories: make([]domain.CategorySummary, 0)}
	// Accumulate the exact decimal database sums before converting to JSON numbers.
	total := new(big.Rat)
	for rows.Next() {
		var category, amount string
		if err := rows.Scan(&category, &amount); err != nil {
			return domain.ReportSummary{}, err
		}
		value, ok := new(big.Rat).SetString(amount)
		if !ok {
			return domain.ReportSummary{}, fmt.Errorf("некорректная сумма отчёта")
		}
		total.Add(total, value)
		number, _ := value.Float64()
		result.Categories = append(result.Categories, domain.CategorySummary{Category: category, Amount: number})
	}
	if err := rows.Err(); err != nil {
		return domain.ReportSummary{}, err
	}
	result.Total, _ = total.Float64()
	return result, nil
}
