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

func SetBudget(b Budget) {
	budgets[b.Category] = b
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

	for i, b := range loaded {
		if strings.TrimSpace(b.Category) == "" {
			return fmt.Errorf("бюджет №%d: категория не должна быть пустой", i+1)
		}
		if b.Limit < 0 {
			return fmt.Errorf("бюджет %q: лимит не должен быть отрицательным", b.Category)
		}
	}
	for _, b := range loaded {
		SetBudget(b)
	}
	return nil
}

func AddTransaction(tx Transaction) error {
	if tx.Amount <= 0 {
		return errors.New("сумма расхода должна быть больше 0")
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
	date := "2026-09-30"
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
	SetBudget(Budget{Category: "Продукты", Limit: 6000})
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
