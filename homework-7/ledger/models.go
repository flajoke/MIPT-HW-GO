package ledger

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

var ErrBudgetExceeded = errors.New("budget exceeded")

type Transaction struct {
	ID          int
	Amount      float64
	Category    string
	Description string
	Date        string
}
type Budget struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}
type Validatable interface{ Validate() error }

func CheckValid(v Validatable) error { return v.Validate() }

// NUMERIC(14,2): at most 12 digits before the decimal point.
func validMoney(value float64) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value >= 1e12 {
		return false
	}
	_, fraction, _ := strings.Cut(strconv.FormatFloat(value, 'f', -1, 64), ".")
	return len(fraction) <= 2
}

func (tx Transaction) Validate() error {
	if !validMoney(tx.Amount) {
		return errors.New("сумма должна быть больше 0, меньше 1000000000000 и содержать не более двух знаков после точки")
	}
	if strings.TrimSpace(tx.Category) == "" {
		return errors.New("категория транзакции не должна быть пустой")
	}
	if _, err := time.Parse("2006-01-02", tx.Date); err != nil {
		return fmt.Errorf("дата должна быть в формате ГГГГ-ММ-ДД: %w", err)
	}
	return nil
}
func (b Budget) Validate() error {
	if !validMoney(b.Limit) {
		return errors.New("лимит должен быть больше 0, меньше 1000000000000 и содержать не более двух знаков после точки")
	}
	if strings.TrimSpace(b.Category) == "" {
		return errors.New("категория бюджета не должна быть пустой")
	}
	return nil
}
