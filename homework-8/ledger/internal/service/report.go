package service

import (
	"context"
	"encoding/json"
	"fmt"
	"homework-8/ledger/internal/domain"
	"log"
)

func (s *Ledger) GetReportSummary(ctx context.Context, from, to string) (domain.ReportSummary, error) {
	if err := domain.ValidateReportRange(from, to); err != nil {
		return domain.ReportSummary{}, &domain.ValidationError{Err: err}
	}
	ctx, cancel := bounded(ctx)
	defer cancel()
	key := ""
	if s.cache != nil {
		version, err := s.cache.Version(ctx)
		if err == nil {
			key = fmt.Sprintf("report:summary:%s:%s:v%s", from, to, version)
			if data, err := s.cache.Get(ctx, key); err == nil {
				var result domain.ReportSummary
				if json.Unmarshal(data, &result) == nil {
					log.Printf("report cache hit: %s", key)
					return result, nil
				}
			}
		} else {
			log.Printf("Redis: кеш недоступен, отчёт читается из PostgreSQL: %v", err)
		}
	}
	log.Printf("report cache miss: %s..%s", from, to)
	result, err := s.reports.Summary(ctx, from, to)
	if err != nil {
		return domain.ReportSummary{}, err
	}

	if key != "" {
		data, err := json.Marshal(result)
		if err == nil {
			if err = s.cache.Set(ctx, key, data); err != nil {
				log.Printf("Redis: запись кеша не удалась: %v", err)
			}
		}
	}
	return result, nil
}
