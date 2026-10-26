package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"time"
)

type CategorySummary struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
}
type ReportSummary struct {
	From       string            `json:"from"`
	To         string            `json:"to"`
	Total      float64           `json:"total"`
	Categories []CategorySummary `json:"categories"`
}

func ValidateReportRange(from, to string) error {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return fmt.Errorf("from должен быть датой YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil {
		return fmt.Errorf("to должен быть датой YYYY-MM-DD")
	}
	if start.After(end) {
		return fmt.Errorf("from не может быть позже to")
	}
	return nil
}
func GetReportSummary(ctx context.Context, from, to string) (ReportSummary, error) {
	if err := ValidateReportRange(from, to); err != nil {
		return ReportSummary{}, err
	}
	if err := ready(); err != nil {
		return ReportSummary{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	key := ""
	if reports != nil {
		version, err := reports.Version(ctx)
		if err == nil {
			key = fmt.Sprintf("report:summary:%s:%s:v%s", from, to, version)
			if data, err := reports.Get(ctx, key); err == nil {
				var result ReportSummary
				if json.Unmarshal(data, &result) == nil {
					log.Printf("report cache hit: %s", key)
					return result, nil
				}
			}
		} else {
			log.Printf("Redis: кеш недоступен, отчёт читается из PostgreSQL: %v", err)
		}
	}
	log.Printf("report cache miss: %s..%s", from, to)
	rows, err := database.QueryContext(ctx, `SELECT category,SUM(amount)::text FROM expenses
 WHERE date BETWEEN $1::date AND $2::date GROUP BY category ORDER BY category`, from, to)
	if err != nil {
		return ReportSummary{}, err
	}
	defer rows.Close()
	result := ReportSummary{From: from, To: to, Categories: make([]CategorySummary, 0)}
	// Accumulate the exact decimal database sums before converting to JSON numbers.
	total := new(big.Rat)
	for rows.Next() {
		var category, amount string
		if err := rows.Scan(&category, &amount); err != nil {
			return ReportSummary{}, err
		}
		value, ok := new(big.Rat).SetString(amount)
		if !ok {
			return ReportSummary{}, fmt.Errorf("некорректная сумма отчёта")
		}
		total.Add(total, value)
		number, _ := value.Float64()
		result.Categories = append(result.Categories, CategorySummary{Category: category, Amount: number})
	}
	if err := rows.Err(); err != nil {
		return ReportSummary{}, err
	}
	result.Total, _ = total.Float64()
	if key != "" {
		data, err := json.Marshal(result)
		if err == nil {
			if err = reports.Set(ctx, key, data); err != nil {
				log.Printf("Redis: запись кеша не удалась: %v", err)
			}
		}
	}
	return result, nil
}
