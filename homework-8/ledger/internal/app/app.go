package app

import (
	"context"
	"errors"
	"fmt"
	"homework-8/ledger/internal/cache"
	"homework-8/ledger/internal/db"
	"homework-8/ledger/internal/repository/pg"
	"homework-8/ledger/internal/service"
	"log"
)

func Open(ctx context.Context) (service.Service, func() error, error) {
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
	result := service.New(budgets, expenses, pg.NewReports(database), pg.NewUnitOfWork(database), cached)
	log.Println("Ledger: подключение к PostgreSQL установлено")
	log.Println("Ledger: подключение к Redis установлено")
	return result, closeFn, nil
}
