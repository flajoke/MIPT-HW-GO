package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"homework-10/ledger/internal/domain"
	"log"
	"math/big"
	"strconv"
	"sync"
	"time"
)

type Options struct {
	BulkWorkDelay time.Duration
	// ReportWorkDelay is zero in normal operation. A cancellable delay makes manual
	// timeout/heartbeat demonstrations repeatable even on a small database.
	ReportWorkDelay   time.Duration
	HeartbeatInterval time.Duration
}

func (s *Ledger) GetReportSummary(ctx context.Context, from, to string) (domain.ReportSummary, error) {
	if err := domain.ValidateReportRange(from, to); err != nil {
		return domain.ReportSummary{}, &domain.ValidationError{Err: err}
	}
	if err := ctx.Err(); err != nil {
		return domain.ReportSummary{}, err
	}
	key := ""
	// Demo calculations bypass the cache so a cache hit cannot hide a timeout.
	if s.cache != nil && s.options.ReportWorkDelay == 0 {
		version, err := s.cache.Version(ctx)
		if err == nil {
			key = fmt.Sprintf("report:summary:%s:%s:v%s", from, to, version)
			if data, err := s.cache.Get(ctx, key); err == nil {
				var result domain.ReportSummary
				if json.Unmarshal(data, &result) == nil {
					if err := ctx.Err(); err != nil {
						return domain.ReportSummary{}, err
					}
					log.Printf("report cache hit: %s", key)
					return result, nil
				}
			}
		} else {
			log.Printf("Redis: кеш недоступен, отчёт читается из PostgreSQL: %v", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return domain.ReportSummary{}, err
	}
	log.Printf("report cache miss: %s..%s", from, to)
	result, err := s.calculateSummary(ctx, from, to)
	if err != nil {
		return domain.ReportSummary{}, err
	}
	if key != "" {
		if data, err := json.Marshal(result); err == nil {
			if err = s.cache.Set(ctx, key, data); err != nil {
				log.Printf("Redis: запись кеша не удалась: %v", err)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return domain.ReportSummary{}, err
	}
	return result, nil
}

func (s *Ledger) calculateSummary(ctx context.Context, from, to string) (domain.ReportSummary, error) {
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	var heartbeat sync.WaitGroup
	heartbeat.Add(1)
	go func() {
		defer heartbeat.Done()
		ticker := time.NewTicker(s.options.HeartbeatInterval)
		defer ticker.Stop()
		defer log.Println("report heartbeat stopped")
		log.Println("report heartbeat started")
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				log.Println("report calculation in progress")
			}
		}
	}()
	defer func() {
		stopHeartbeat()
		heartbeat.Wait()
		if err := ctx.Err(); err != nil {
			log.Printf("report calculation canceled: %v", err)
		}
	}()
	categories, err := s.reports.ListCategories(ctx, from, to)
	if err != nil {
		return domain.ReportSummary{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.ReportSummary{}, err
	}
	type categoryResult struct {
		amount float64
		err    error
	}
	results := make([]categoryResult, len(categories))
	workersCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()
	var workers sync.WaitGroup
	for index, category := range categories {
		workers.Add(1)
		go func(index int, category string) {
			defer workers.Done()
			if delay := s.options.ReportWorkDelay; delay > 0 {
				timer := time.NewTimer(delay)
				defer timer.Stop()
				select {
				case <-workersCtx.Done():
					results[index].err = workersCtx.Err()
					return
				case <-timer.C:
				}
			}
			if err := workersCtx.Err(); err != nil {
				results[index].err = err
				return
			}
			results[index].amount, results[index].err = s.reports.SumForPeriod(workersCtx, category, from, to)
			if results[index].err != nil {
				cancelWorkers()
			}
		}(index, category)
	}
	workers.Wait()
	log.Println("report workers finished")
	if err := ctx.Err(); err != nil {
		return domain.ReportSummary{}, err
	}
	// Prefer the original repository error over cancellation of sibling workers.
	for _, result := range results {
		if result.err != nil && !errors.Is(result.err, context.Canceled) && !errors.Is(result.err, context.DeadlineExceeded) {
			return domain.ReportSummary{}, result.err
		}
	}
	for _, result := range results {
		if result.err != nil {
			return domain.ReportSummary{}, result.err
		}
	}
	report := domain.ReportSummary{From: from, To: to, Categories: make([]domain.CategorySummary, 0, len(categories))}
	total := new(big.Rat)
	// Each worker wrote only its own slot. Read/merge only after Wait.
	for index, category := range categories {
		amount := results[index].amount
		value, ok := new(big.Rat).SetString(strconv.FormatFloat(amount, 'f', -1, 64))
		if !ok {
			return domain.ReportSummary{}, fmt.Errorf("некорректная сумма отчёта")
		}
		total.Add(total, value)
		report.Categories = append(report.Categories, domain.CategorySummary{Category: category, Amount: amount})
	}
	report.Total, _ = total.Float64()
	return report, nil
}
