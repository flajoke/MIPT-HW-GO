package service

import (
	"context"
	"errors"
	"fmt"
	"homework-10/ledger/internal/domain"
	"log"
	"sort"
	"sync"
	"time"
)

const DefaultBulkWorkers = 4
const MaxBulkWorkers = 32

type importJob struct {
	index       int
	transaction domain.Transaction
}
type importResult struct {
	index       int
	err         error
	interrupted bool
}
type importSummary struct {
	mu     sync.Mutex
	result domain.BulkResult
}

func (s *importSummary) collect(result importResult) {
	if result.interrupted {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if result.err == nil {
		s.result.Accepted++
		return
	}
	s.result.Rejected++
	var validation *domain.ValidationError
	message := "internal server error"
	switch {
	case errors.As(result.err, &validation):
		message = validation.Error()
	case errors.Is(result.err, domain.ErrBudgetExceeded):
		message = "budget exceeded"
	case errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded):
		message = "operation timeout"
	default:
		log.Printf("bulk item %d failed: %v", result.index, result.err)
	}
	s.result.Errors = append(s.result.Errors, domain.BulkError{Index: result.index, Error: message})
}
func (s *importSummary) snapshot(total int) domain.BulkResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.result
	result.Unprocessed = total - result.Accepted - result.Rejected
	sort.Slice(result.Errors, func(i, j int) bool { return result.Errors[i].Index < result.Errors[j].Index })
	return result
}
func (s *Ledger) ImportTransactions(ctx context.Context, input []domain.Transaction, workers int) (domain.BulkResult, error) {
	summary := &importSummary{result: domain.BulkResult{Errors: make([]domain.BulkError, 0)}}
	if workers < 1 || workers > MaxBulkWorkers {
		return summary.snapshot(len(input)), &domain.ValidationError{Err: fmt.Errorf("workers должен быть от 1 до %d", MaxBulkWorkers)}
	}
	if err := ctx.Err(); err != nil {
		return summary.snapshot(len(input)), err
	}
	if len(input) == 0 {
		return summary.snapshot(0), nil
	}
	if workers > len(input) {
		workers = len(input)
	}
	jobs := make(chan importJob, workers)
	results := make(chan importResult, workers)
	var collector sync.WaitGroup
	collector.Add(1)
	go func() {
		defer collector.Done()
		for result := range results {
			summary.collect(result)
		}
	}()
	var pool sync.WaitGroup
	pool.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer pool.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				var job importJob
				select {
				case <-ctx.Done():
					return
				case next, ok := <-jobs:
					if !ok {
						return
					}
					job = next
				}
				if ctx.Err() != nil {
					return
				}
				if delay := s.options.BulkWorkDelay; delay > 0 {
					timer := time.NewTimer(delay)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
				}
				if ctx.Err() != nil {
					return
				}
				_, err := s.AddTransaction(ctx, job.transaction)
				var validation *domain.ValidationError
				interrupted := err != nil && ctx.Err() != nil && !errors.As(err, &validation) && !errors.Is(err, domain.ErrBudgetExceeded)
				// Never drop an outcome after a write: the collector outlives all workers,
				// even if cancellation happened immediately after a successful commit.
				results <- importResult{index: job.index, err: err, interrupted: interrupted}
			}
		}()
	}
dispatch:
	for index, value := range input {
		if ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- importJob{index: index, transaction: value}:
		}
	}
	close(jobs)
	pool.Wait()
	close(results)
	collector.Wait()
	result := summary.snapshot(len(input))
	log.Printf("bulk import finished: accepted=%d rejected=%d unprocessed=%d", result.Accepted, result.Rejected, result.Unprocessed)
	return result, ctx.Err()
}
