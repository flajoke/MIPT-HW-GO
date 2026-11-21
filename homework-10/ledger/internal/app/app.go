package app

import (
	"context"
	"errors"
	"fmt"
	"homework-10/ledger/internal/cache"
	"homework-10/ledger/internal/db"
	"homework-10/ledger/internal/repository/pg"
	"homework-10/ledger/internal/service"
	"log"
	"os"
	"time"
)

func Open(ctx context.Context) (service.Service, func() error, error) {
	delay := time.Duration(0)
	if raw := os.Getenv("REPORT_WORK_DELAY"); raw != "" {
		var err error
		delay, err = time.ParseDuration(raw)
		if err != nil || delay < 0 {
			return nil, nil, fmt.Errorf("REPORT_WORK_DELAY должен быть длительностью >= 0, например 800ms")
		}
	}
	bulkDelay := time.Duration(0)
	if raw := os.Getenv("BULK_WORK_DELAY"); raw != "" {
		var err error
		bulkDelay, err = time.ParseDuration(raw)
		if err != nil || bulkDelay < 0 {
			return nil, nil, fmt.Errorf("BULK_WORK_DELAY должен быть длительностью >= 0, например 200ms")
		}
	}
	database, err := db.Open(ctx)
	if err != nil {
		return nil, nil, err
	}
	cached, err := cache.Open(ctx)
	if err != nil {
		database.Close()
		return nil, nil, err
	}
	closeFn := func() error { return errors.Join(cached.Close(), database.Close()) }
	if err := db.CheckSchema(ctx, database); err != nil {
		closeFn()
		return nil, nil, fmt.Errorf("схема БД недоступна; выполните goose up: %w", err)
	}
	budgets := pg.NewBudgets(database)
	expenses := pg.NewExpenses(database)
	result := service.New(budgets, expenses, pg.NewReports(database), pg.NewUnitOfWork(database), cached, service.Options{ReportWorkDelay: delay, BulkWorkDelay: bulkDelay})
	log.Println("Ledger: подключение к PostgreSQL установлено")
	log.Println("Ledger: подключение к Redis установлено")
	return result, closeFn, nil
}
