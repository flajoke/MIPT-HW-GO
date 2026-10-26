package ledger

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Explicit opt-in: only an already migrated, disposable *_test database is accepted.
func TestPostgresRedis(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a migrated *_test database for PostgreSQL/Redis integration tests")
	}
	t.Setenv("DATABASE_URL", dsn)
	redisDB := os.Getenv("TEST_REDIS_DB")
	if redisDB == "" {
		redisDB = "14"
	}
	t.Setenv("REDIS_DB", redisDB)
	ctx := context.Background()
	if err := Init(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Close() })
	var name string
	if err := database.QueryRow("SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("integration tests require a database name ending in _test")
	}
	reset := func(t *testing.T) {
		t.Helper()
		if _, err := database.Exec("TRUNCATE expenses,budgets RESTART IDENTITY"); err != nil {
			t.Fatal(err)
		}
		if err := reports.Invalidate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { reset(t) })
	set := func(t *testing.T, limit float64) {
		t.Helper()
		if err := SetBudget(Budget{"food", limit}); err != nil {
			t.Fatal(err)
		}
	}
	add := func(t *testing.T, amount float64, category, date string) Transaction {
		t.Helper()
		value, err := AddTransaction(Transaction{Amount: amount, Category: category, Date: date, Description: "lunch"})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	t.Run("budget_upsert_and_limit", func(t *testing.T) {
		reset(t)
		set(t, 500)
		set(t, 600)
		list, err := ListBudgets()
		if err != nil || len(list) != 1 || list[0].Limit != 600 {
			t.Fatalf("budgets=%+v err=%v", list, err)
		}
		saved := add(t, 450, "food", "2026-10-25")
		if saved.ID != 1 {
			t.Fatal(saved)
		}
		if _, err := AddTransaction(Transaction{Amount: 151, Category: "food", Date: "2026-10-25"}); !errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("expected budget exceeded: %v", err)
		}
		txs, err := ListTransactions()
		if err != nil || len(txs) != 1 {
			t.Fatal(txs, err)
		}
		add(t, 150, "food", "2026-10-25")
		add(t, 1000, "other", "2026-10-24")
		txs, err = ListTransactions()
		if err != nil || len(txs) != 3 || txs[0].ID != 2 || txs[1].ID != 1 {
			t.Fatal(txs, err)
		}
	})
	t.Run("decimal_exact_limit", func(t *testing.T) {
		reset(t)
		set(t, 0.30)
		add(t, 0.10, "food", "2026-10-25")
		add(t, 0.20, "food", "2026-10-25")
		if _, err := AddTransaction(Transaction{Amount: 0.01, Category: "food", Date: "2026-10-25"}); !errors.Is(err, ErrBudgetExceeded) {
			t.Fatal(err)
		}
	})
	t.Run("invalid_not_saved", func(t *testing.T) {
		reset(t)
		if err := SetBudget(Budget{"food", 0}); err == nil {
			t.Fatal("expected validation error")
		}
		if _, err := AddTransaction(Transaction{}); err == nil {
			t.Fatal("expected validation error")
		}
		txs, err := ListTransactions()
		if err != nil || len(txs) != 0 {
			t.Fatal(txs, err)
		}
	})
	t.Run("concurrent_limit", func(t *testing.T) {
		reset(t)
		set(t, 100)
		var wg sync.WaitGroup
		out := make(chan error, 10)
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := AddTransaction(Transaction{Amount: 20, Category: "food", Date: "2026-10-25"})
				out <- err
			}()
		}
		wg.Wait()
		close(out)
		ok, exceeded := 0, 0
		for err := range out {
			if err == nil {
				ok++
			} else if errors.Is(err, ErrBudgetExceeded) {
				exceeded++
			} else {
				t.Fatal(err)
			}
		}
		if ok != 5 || exceeded != 5 {
			t.Fatalf("ok=%d exceeded=%d", ok, exceeded)
		}
	})
	t.Run("restart_persists", func(t *testing.T) {
		reset(t)
		set(t, 500)
		saved := add(t, 100, "food", "2026-10-25")
		if err := Close(); err != nil {
			t.Fatal(err)
		}
		if err := Init(ctx); err != nil {
			t.Fatal(err)
		}
		txs, err := ListTransactions()
		if err != nil || len(txs) != 1 || txs[0] != saved {
			t.Fatal(txs, err)
		}
		bs, err := ListBudgets()
		if err != nil || len(bs) != 1 || bs[0].Limit != 500 {
			t.Fatal(bs, err)
		}
	})
	t.Run("report_cache_ttl_and_invalidation", func(t *testing.T) {
		reset(t)
		add(t, 100, "food", "2026-10-25")
		add(t, 200, "transport", "2026-10-26")
		add(t, 900, "outside", "2025-01-01")
		first, err := GetReportSummary(ctx, "2026-10-25", "2026-10-26")
		if err != nil || first.Total != 300 || len(first.Categories) != 2 {
			t.Fatal(first, err)
		}
		version, err := reports.Version(ctx)
		if err != nil {
			t.Fatal(err)
		}
		key := "report:summary:2026-10-25:2026-10-26:v" + version
		addr := os.Getenv("REDIS_ADDR")
		if addr == "" {
			addr = "localhost:6379"
		}
		index := 14
		if redisDB != "14" {
			var err error
			index, err = strconv.Atoi(redisDB)
			if err != nil {
				t.Fatal(err)
			}
		}
		client := redis.NewClient(&redis.Options{Addr: addr, DB: index, Password: os.Getenv("REDIS_PASSWORD")})
		defer client.Close()
		ttl, err := client.TTL(ctx, key).Result()
		if err != nil || ttl <= 0 || ttl > 30*time.Second {
			t.Fatal(ttl, err)
		}
		// Direct SQL bypasses invalidation: the second read proves it came from Redis.
		if _, err := database.Exec("UPDATE expenses SET amount=150 WHERE category='food'"); err != nil {
			t.Fatal(err)
		}
		second, err := GetReportSummary(ctx, "2026-10-25", "2026-10-26")
		if err != nil || second.Total != 300 {
			t.Fatal(second, err)
		}
		if err := client.PExpire(ctx, key, time.Millisecond).Err(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
		third, err := GetReportSummary(ctx, "2026-10-25", "2026-10-26")
		if err != nil || third.Total != 350 {
			t.Fatal(third, err)
		}
		add(t, 25, "food", "2026-10-25")
		fourth, err := GetReportSummary(ctx, "2026-10-25", "2026-10-26")
		if err != nil || fourth.Total != 375 {
			t.Fatal(fourth, err)
		}
		empty, err := GetReportSummary(ctx, "2020-01-01", "2020-01-02")
		if err != nil || empty.Total != 0 || empty.Categories == nil || len(empty.Categories) != 0 {
			t.Fatal(empty, err)
		}
	})
}
