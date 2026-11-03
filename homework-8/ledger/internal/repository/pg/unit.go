package pg

import (
	"context"
	"database/sql"
	"homework-8/ledger/internal/domain"
)

type UnitOfWork struct{ db *sql.DB }

func NewUnitOfWork(db *sql.DB) *UnitOfWork { return &UnitOfWork{db: db} }

var _ domain.UnitOfWork = (*UnitOfWork)(nil)

func (u *UnitOfWork) WithinCategory(ctx context.Context, category string, fn func(domain.BudgetRepository, domain.ExpenseRepository) error) error {
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Shared across processes; the lock also covers categories without a budget row.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", category); err != nil {
		return err
	}
	if err = fn(NewBudgets(tx), NewExpenses(tx)); err != nil {
		return err
	}
	return tx.Commit()
}
