package domain

import (
	"fmt"
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
