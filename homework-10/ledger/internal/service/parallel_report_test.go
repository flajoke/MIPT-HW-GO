package service

import (
	"bytes"
	"context"
	"errors"
	"homework-10/ledger/internal/domain"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type reportProbe struct {
	categories   []string
	sums         map[string]float64
	started      chan string
	release      chan struct{}
	active       atomic.Int32
	failCategory string
	failure      error
}

func (p *reportProbe) ListCategories(ctx context.Context, from, to string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p.categories, nil
}
func (p *reportProbe) SumForPeriod(ctx context.Context, category, from, to string) (float64, error) {
	p.active.Add(1)
	defer p.active.Add(-1)
	p.started <- category
	if category == p.failCategory {
		return 0, p.failure
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-p.release:
		return p.sums[category], nil
	}
}
func probe(categories ...string) *reportProbe {
	return &reportProbe{categories: categories, sums: map[string]float64{}, started: make(chan string, len(categories)), release: make(chan struct{})}
}

type outcome struct {
	report domain.ReportSummary
	err    error
}

func calculate(s *Ledger, ctx context.Context) <-chan outcome {
	done := make(chan outcome, 1)
	go func() { r, err := s.GetReportSummary(ctx, "2026-11-01", "2026-11-30"); done <- outcome{r, err} }()
	return done
}
func await(t *testing.T, done <-chan outcome) outcome {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(time.Second):
		t.Fatal("report workers did not stop")
		return outcome{}
	}
}
func TestParallelReportStartsAllCategories(t *testing.T) {
	s, _ := fixture()
	p := probe("food", "transport", "zero")
	p.sums = map[string]float64{"food": .10, "transport": .20, "zero": 0}
	s.reports = p
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := calculate(s, ctx)
	// All workers must start before any of them can finish. Serial execution fails.
	for range p.categories {
		select {
		case <-p.started:
		case <-time.After(time.Second):
			cancel()
			await(t, done)
			t.Fatal("categories were not processed concurrently")
		}
	}
	close(p.release)
	result := await(t, done)
	if result.err != nil || result.report.Total != .30 || len(result.report.Categories) != 3 {
		t.Fatal(result)
	}
	if p.active.Load() != 0 {
		t.Fatal("workers left running after return")
	}
}
func TestParallelReportDeadline(t *testing.T) {
	s, _ := fixture()
	p := probe("food", "transport")
	s.reports = p
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	result := await(t, calculate(s, ctx))
	if !errors.Is(result.err, context.DeadlineExceeded) {
		t.Fatal(result.err)
	}
	if p.active.Load() != 0 {
		t.Fatal("workers left running after deadline")
	}
}
func TestParallelReportRepositoryErrorCancelsSiblings(t *testing.T) {
	s, _ := fixture()
	p := probe("blocked", "failure")
	sentinel := errors.New("query failed")
	p.failCategory = "failure"
	p.failure = sentinel
	s.reports = p
	result := await(t, calculate(s, context.Background()))
	if !errors.Is(result.err, sentinel) {
		t.Fatal("original error was lost", result.err)
	}
	if p.active.Load() != 0 {
		t.Fatal("sibling workers did not stop")
	}
}

type heartbeatLog struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	tick   chan struct{}
}

func (w *heartbeatLog) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buffer.Write(data)
	if bytes.Contains(data, []byte("in progress")) {
		select {
		case w.tick <- struct{}{}:
		default:
		}
	}
	return n, err
}
func (w *heartbeatLog) String() string { w.mu.Lock(); defer w.mu.Unlock(); return w.buffer.String() }
func captureHeartbeat(t *testing.T) *heartbeatLog {
	t.Helper()
	w := &heartbeatLog{tick: make(chan struct{}, 1)}
	old := log.Writer()
	log.SetOutput(io.Writer(w))
	t.Cleanup(func() { log.SetOutput(old) })
	return w
}
func TestHeartbeatStopsOnCancel(t *testing.T) {
	logs := captureHeartbeat(t)
	s, _ := fixture()
	s.options.HeartbeatInterval = 2 * time.Millisecond
	p := probe("food")
	s.reports = p
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := calculate(s, ctx)
	select {
	case <-logs.tick:
	case <-time.After(time.Second):
		cancel()
		await(t, done)
		t.Fatal("heartbeat did not tick")
	}
	cancel()
	result := await(t, done)
	if !errors.Is(result.err, context.Canceled) || p.active.Load() != 0 {
		t.Fatal(result.err, p.active.Load())
	}
	if !strings.Contains(logs.String(), "heartbeat stopped") || !strings.Contains(logs.String(), "calculation canceled") {
		t.Fatal(logs.String())
	}
}
func TestDemoDelayHonorsContext(t *testing.T) {
	s, _ := fixture()
	p := probe("food")
	s.reports = p
	s.options.ReportWorkDelay = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := await(t, calculate(s, ctx))
	if !errors.Is(result.err, context.DeadlineExceeded) {
		t.Fatal(result.err)
	}
	if len(p.started) != 0 {
		t.Fatal("SQL started after cancellation during demo delay")
	}
}
func TestReportEmptyAndAlreadyCanceled(t *testing.T) {
	s, _ := fixture()
	p := probe()
	s.reports = p
	r, err := s.GetReportSummary(context.Background(), "2026-11-01", "2026-11-30")
	if err != nil || r.Total != 0 || r.Categories == nil || len(r.Categories) != 0 {
		t.Fatal(r, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.GetReportSummary(ctx, "2026-11-01", "2026-11-30"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
