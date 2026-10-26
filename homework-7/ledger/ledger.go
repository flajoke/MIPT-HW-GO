package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"homework-7/ledger/internal/cache"
	"homework-7/ledger/internal/db"
	"log"
	"time"
)

// Init is called once before serving requests; Close runs after the server stops.
var database *sql.DB
var reports *cache.Cache

func Init(ctx context.Context) error {
	if database != nil {
		return errors.New("Ledger уже инициализирован")
	}
	opened, err := db.Open(ctx)
	if err != nil {
		return err
	}
	cached, err := cache.Open(ctx)
	if err != nil {
		opened.Close()
		return err
	}
	// A successful Ping does not guarantee that migrations were applied.
	rows, err := opened.QueryContext(ctx, "SELECT id, category, limit_amount FROM budgets LIMIT 0")
	if err != nil {
		opened.Close()
		cached.Close()
		return fmt.Errorf("схема budgets недоступна; выполните goose up: %w", err)
	}
	rows.Close()
	rows, err = opened.QueryContext(ctx, "SELECT id, amount, category, description, date FROM expenses LIMIT 0")
	if err != nil {
		opened.Close()
		cached.Close()
		return fmt.Errorf("схема expenses недоступна; выполните goose up: %w", err)
	}
	rows.Close()
	database = opened
	reports = cached
	log.Println("Ledger: подключение к PostgreSQL установлено")
	log.Println("Ledger: подключение к Redis установлено")
	return nil
}
func Close() error {
	var err error
	if reports != nil {
		err = reports.Close()
		reports = nil
	}
	if database != nil {
		err = errors.Join(err, database.Close())
		database = nil
	}
	return err
}
func operationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
func ready() error {
	if database == nil {
		return errors.New("Ledger не инициализирован")
	}
	return nil
}

func SetBudget(b Budget) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if err := ready(); err != nil {
		return err
	}
	ctx, cancel := operationContext()
	defer cancel()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockCategory(ctx, tx, b.Category); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO budgets(category,limit_amount) VALUES($1,$2)
 ON CONFLICT(category) DO UPDATE SET limit_amount=EXCLUDED.limit_amount`, b.Category, b.Limit)
	if err != nil {
		return fmt.Errorf("сохранение бюджета: %w", err)
	}
	return tx.Commit()
}
func ListBudgets() ([]Budget, error) {
	if err := ready(); err != nil {
		return nil, err
	}
	ctx, cancel := operationContext()
	defer cancel()
	rows, err := database.QueryContext(ctx, "SELECT category,limit_amount FROM budgets ORDER BY category")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Budget, 0)
	for rows.Next() {
		var b Budget
		if err := rows.Scan(&b.Category, &b.Limit); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

// The same transaction-scoped lock protects additions and budget updates,
// including categories that do not yet have a budget row.
func lockCategory(ctx context.Context, tx *sql.Tx, category string) error {
	_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", category)
	return err
}
func AddTransaction(value Transaction) (Transaction, error) {
	if err := value.Validate(); err != nil {
		return Transaction{}, err
	}
	if err := ready(); err != nil {
		return Transaction{}, err
	}
	ctx, cancel := operationContext()
	defer cancel()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return Transaction{}, err
	}
	defer tx.Rollback()
	if err = lockCategory(ctx, tx, value.Category); err != nil {
		return Transaction{}, err
	}
	var exceeded bool
	// Money is added and compared as PostgreSQL NUMERIC, without floating-point arithmetic.
	err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT
 (SELECT COALESCE(SUM(amount),0) FROM expenses WHERE category=$1) + $2::numeric > limit_amount
 FROM budgets WHERE category=$1),false)`, value.Category, value.Amount).Scan(&exceeded)
	if err != nil {
		return Transaction{}, fmt.Errorf("проверка бюджета: %w", err)
	}
	if exceeded {
		return Transaction{}, ErrBudgetExceeded
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO expenses(amount,category,description,date)
 VALUES($1,$2,$3,$4) RETURNING id`, value.Amount, value.Category, value.Description, value.Date).Scan(&value.ID)
	if err != nil {
		return Transaction{}, fmt.Errorf("сохранение транзакции: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return Transaction{}, err
	}
	if reports != nil {
		if err := reports.Invalidate(ctx); err != nil {
			log.Printf("Redis: не удалось обновить версию отчётов: %v", err)
		}
	}
	return value, nil
}
func ListTransactions() ([]Transaction, error) {
	if err := ready(); err != nil {
		return nil, err
	}
	ctx, cancel := operationContext()
	defer cancel()
	rows, err := database.QueryContext(ctx, "SELECT id,amount,category,COALESCE(description,''),date FROM expenses ORDER BY date DESC,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Transaction, 0)
	for rows.Next() {
		var value Transaction
		var date time.Time
		if err := rows.Scan(&value.ID, &value.Amount, &value.Category, &value.Description, &date); err != nil {
			return nil, err
		}
		value.Date = date.Format("2006-01-02")
		result = append(result, value)
	}
	return result, rows.Err()
}
