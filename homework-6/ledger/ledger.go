package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type Transaction struct {
	ID          int
	Amount      int
	Category    string
	Description string
	Date        string
}

type Budget struct {
	Category string `json:"category"`
	Limit    int    `json:"limit"`
}

var mu sync.RWMutex

var transactions = make([]Transaction, 0)
var budgets = make(map[string]Budget)
var ErrBudgetExceeded = errors.New("budget exceeded")

// Validatable задаёт общий контракт проверки данных.
type Validatable interface {
	Validate() error
}

func (tx Transaction) Validate() error {
	if tx.Amount <= 0 {
		return errors.New("сумма расхода должна быть больше 0")
	}
	if strings.TrimSpace(tx.Category) == "" {
		return errors.New("категория транзакции не должна быть пустой")
	}
	if _, err := time.Parse("2006-01-02", tx.Date); err != nil {
		return fmt.Errorf("дата транзакции должна быть в формате ГГГГ-ММ-ДД: %w", err)
	}
	return nil
}

func (b Budget) Validate() error {
	if b.Limit <= 0 {
		return errors.New("лимит бюджета должен быть больше 0")
	}
	if strings.TrimSpace(b.Category) == "" {
		return errors.New("категория бюджета не должна быть пустой")
	}
	return nil
}

func CheckValid(v Validatable) error {
	return v.Validate()
}

func SetBudget(b Budget) error {
	if err := b.Validate(); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	budgets[b.Category] = b
	return nil
}

func LoadBudgets(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("Не удалось прочитать бюджеты: %w", err)
	}

	var loaded []Budget
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("Не удалось разобрать JSON бюджетов: %w", err)
	}
	if loaded == nil {
		return errors.New("ожидался JSON-массив бюджетов, например []")
	}

	// Сначала проверяем весь файл, чтобы не записать часть бюджетов при ошибке.
	for i, b := range loaded {
		if err := CheckValid(b); err != nil {
			return fmt.Errorf("бюджет №%d: %w", i+1, err)
		}
	}
	for _, b := range loaded {
		if err := SetBudget(b); err != nil {
			return err
		}
	}
	return nil
}

func AddTransaction(tx Transaction) (Transaction, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := tx.Validate(); err != nil {
		return Transaction{}, err
	}
	if budget, exists := budgets[tx.Category]; exists {
		spent := 0
		for _, saved := range transactions {
			if saved.Category == tx.Category {
				if saved.Amount > budget.Limit-spent {
					return Transaction{}, fmt.Errorf("%w: расходы категории %q уже выше лимита %d руб.",
						ErrBudgetExceeded, tx.Category, budget.Limit)
				}
				spent += saved.Amount
			}
		}
		remaining := budget.Limit - spent
		if tx.Amount > remaining {
			return Transaction{}, fmt.Errorf("%w: категория %q, доступно %d руб., запрошено %d руб.",
				ErrBudgetExceeded, tx.Category, remaining, tx.Amount)
		}
	}
	tx.ID = len(transactions) + 1
	transactions = append(transactions, tx)
	return tx, nil
}

func ListTransactions() []Transaction {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]Transaction, len(transactions))
	copy(result, transactions)
	return result
}

func ListBudgets() []Budget {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]Budget, 0, len(budgets))
	for _, b := range budgets {
		result = append(result, b)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Category < result[j].Category })
	return result
}

// Reset очищает хранилище в памяти, в том числе счётчик ID.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	transactions = make([]Transaction, 0)
	budgets = make(map[string]Budget)
}
