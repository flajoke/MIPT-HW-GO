package main

import (
	"errors"
	"fmt"
	"log"
)

type Transaction struct {
	ID          int
	Amount      int
	Category    string
	Description string
	Date        string
}

var transactions = make([]Transaction, 0)

func AddTransaction(tx Transaction) error {
	if tx.Amount == 0 {
		return errors.New("сумма транзакции не должна быть равна 0")
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

func main() {
	fmt.Println("Ledger server started")

	examples := []Transaction{
		{
			Amount:      1250,
			Category:    "Продукты",
			Description: "Покупка продуктов",
			Date:        "2026-09-27",
		},
		{
			Amount:      70,
			Category:    "Транспорт",
			Description: "Поездка на автобусе",
			Date:        "2026-09-27",
		},
		{
			Amount:      500,
			Category:    "Связь",
			Description: "Оплата мобильной связи",
			Date:        "2026-09-27",
		},
	}
	for _, tx := range examples {
		if err := AddTransaction(tx); err != nil {
			log.Fatal(err)
		}
	}

	err := AddTransaction(Transaction{Amount: 0, Category: "Проверка"})
	if err != nil {
		fmt.Println("Ожидаемая ошибка:", err)
	}
	saved := ListTransactions()
	fmt.Println("Список транзакций:")

	for _, tx := range saved {
		fmt.Printf("ID: %d | Сумма: %d руб. | Категория: %s | Описание: %s | Дата: %s\n",
			tx.ID, tx.Amount, tx.Category, tx.Description, tx.Date)
	}
	fmt.Printf("Всего сохранено транзакций: %d\n", len(saved))
}
