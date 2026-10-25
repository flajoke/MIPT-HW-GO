package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
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

func AddTransaction(tx Transaction) error {
	if err := tx.Validate(); err != nil {
		return err
	}
	if budget, exists := budgets[tx.Category]; exists {
		spent := 0
		for _, saved := range transactions {
			if saved.Category == tx.Category {
				if saved.Amount > budget.Limit-spent {
					return fmt.Errorf("%w: расходы категории %q уже выше лимита %d руб.",
						ErrBudgetExceeded, tx.Category, budget.Limit)
				}
				spent += saved.Amount
			}
		}
		remaining := budget.Limit - spent
		if tx.Amount > remaining {
			return fmt.Errorf("%w: категория %q, доступно %d руб., запрошено %d руб.",
				ErrBudgetExceeded, tx.Category, remaining, tx.Amount)
		}
	}
	tx.ID = len(transactions) + 1
	transactions = append(transactions, tx)
	return nil
}

func ListTransactions() []Transaction {
	result := make([]Transaction, len(transactions))
	copy(result, transactions)
	return result
}

func tryAddTransaction(tx Transaction) {
	if err := AddTransaction(tx); err != nil {
		fmt.Printf("Отказ (%s, %d руб.): %v\n", tx.Category, tx.Amount, err)
		return
	}
	fmt.Printf("Добавлено: %s, %d руб.\n", tx.Category, tx.Amount)
}

func main() {
	fmt.Println("Ledger service started")

	file, err := os.Open("budgets.json")
	if err != nil {
		log.Fatalf("не удалось открыть budgets.json: %v", err)
	}

	loadErr := LoadBudgets(bufio.NewReader(file))
	closeErr := file.Close()
	if loadErr != nil {
		log.Fatal(loadErr)
	}
	if closeErr != nil {
		log.Fatalf("не удалось закрыть budgets.json: %v", closeErr)
	}

	fmt.Printf("Загружено бюджетов: %d\n", len(budgets))
	date := "2026-10-25"
	fmt.Println("\nПроверка разных типов через Validatable:")
	checks := []Validatable{
		Transaction{Amount: 100, Category: "Продукты", Date: date},
		Budget{Category: "Продукты", Limit: 5000},
		Transaction{Amount: -1, Category: "Продукты", Date: date},
		Transaction{Amount: 100, Category: "   ", Date: date},
		Transaction{Amount: 100, Category: "Продукты", Date: "2026-02-30"},
		Budget{Category: "Продукты", Limit: 0},
		Budget{Category: "", Limit: 100},
	}
	for _, value := range checks {
		if err := CheckValid(value); err != nil {
			fmt.Printf("%T: ошибка: %v\n", value, err)
		} else {
			fmt.Printf("%T: данные корректны\n", value)
		}
	}
	fmt.Println()
	examples := []Transaction{
		{Amount: 3000, Category: "Продукты", Description: "Покупка продуктов", Date: date},
		{Amount: 2500, Category: "Продукты", Description: "Превышение бюджета", Date: date},
		{Amount: 2000, Category: "Продукты", Description: "Ровно до лимита", Date: date},
		{Amount: 800, Category: "Транспорт", Description: "Оплата проезда", Date: date},
		{Amount: 700, Category: "Связь", Description: "Категория без бюджета", Date: date},
		{Amount: 0, Category: "Продукты", Description: "Проверка нулевой суммы", Date: date},
	}
	for _, tx := range examples {
		tryAddTransaction(tx)
	}

	fmt.Println("\nУвеличиваем бюджет категории Продукты до 6000 руб.")
	if err := SetBudget(Budget{Category: "Продукты", Limit: 6000}); err != nil {
		log.Fatal(err)
	}
	tryAddTransaction(Transaction{
		Amount:      1000,
		Category:    "Продукты",
		Description: "Покупка после изменения бюджета",
		Date:        date,
	})

	fmt.Println("\nСохранённые транзакции:")
	saved := ListTransactions()
	for _, tx := range saved {
		fmt.Printf("ID: %d | %s | %d руб. | %s | %s\n",
			tx.ID, tx.Category, tx.Amount, tx.Date, tx.Description)
	}
	fmt.Printf("Всего сохранено транзакций: %d\n", len(saved))
}
