// Package ledger is the public entry point for Gateway.
package ledger

import (
	"context"
	"homework-10/ledger/internal/app"
	"homework-10/ledger/internal/domain"
	"homework-10/ledger/internal/service"
)

// Public aliases allow a separate Gateway module to use domain types without
// importing Ledger's internal packages.
type Transaction = domain.Transaction
type Budget = domain.Budget
type ReportSummary = domain.ReportSummary
type CategorySummary = domain.CategorySummary
type ValidationError = domain.ValidationError
type BulkResult = domain.BulkResult
type BulkError = domain.BulkError
type Service = service.Service

const DefaultBulkWorkers = service.DefaultBulkWorkers
const MaxBulkWorkers = service.MaxBulkWorkers

var ErrBudgetExceeded = domain.ErrBudgetExceeded

func Open(ctx context.Context) (Service, func() error, error) { return app.Open(ctx) }
