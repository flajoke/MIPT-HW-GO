package ledger

import (
	"context"
	"database/sql"
	"errors"
	"github.com/redis/go-redis/v9"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPostgresRedis(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a migrated, disposable *_test database")
	}
	t.Setenv("DATABASE_URL", dsn)
	index := 14
	if raw := os.Getenv("TEST_REDIS_DB"); raw != "" {
		var err error
		index, err = strconv.Atoi(raw)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("REDIS_DB", strconv.Itoa(index))
	ctx := context.Background()
	service, closeFn, err := Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeFn != nil {
			closeFn()
		}
	})
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var name string
	if err := database.QueryRow("SELECT current_database()").Scan(&name); err != nil || !strings.HasSuffix(name, "_test") {
		t.Fatal("requires a disposable *_test database", err)
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	cache := redis.NewClient(&redis.Options{Addr: addr, DB: index, Password: os.Getenv("REDIS_PASSWORD")})
	defer cache.Close()
	clear := func(t *testing.T) {
		t.Helper()
		if _, err := database.Exec("TRUNCATE expenses,budgets RESTART IDENTITY"); err != nil {
			t.Fatal(err)
		}
		if err := cache.Incr(ctx, "report:version").Err(); err != nil {
			t.Fatal(err)
		}
	}
	defer clear(t)
	set := func(t *testing.T, limit float64) {
		t.Helper()
		if err := service.SetBudget(ctx, Budget{Category: "food", Limit: limit}); err != nil {
			t.Fatal(err)
		}
	}
	add := func(t *testing.T, amount float64) Transaction {
		t.Helper()
		v, err := service.AddTransaction(ctx, Transaction{Amount: amount, Category: "food", Date: "2026-10-25", Description: "lunch"})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	t.Run("upsert_and_limit", func(t *testing.T) {
		clear(t)
		set(t, 500)
		set(t, 600)
		list, err := service.ListBudgets(ctx)
		if err != nil || len(list) != 1 || list[0].Limit != 600 {
			t.Fatal(list, err)
		}
		first := add(t, 450)
		if first.ID != 1 {
			t.Fatal(first)
		}
		before, err := service.ListTransactions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.AddTransaction(ctx, Transaction{Amount: 151, Category: "food", Date: "2026-10-25"}); !errors.Is(err, ErrBudgetExceeded) {
			t.Fatal(err)
		}
		after, err := service.ListTransactions(ctx)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("rejected operation changed storage", err)
		}
		add(t, 150)
	})
	t.Run("decimal_exact_limit", func(t *testing.T) {
		clear(t)
		set(t, .30)
		add(t, .10)
		add(t, .20)
		if _, err := service.AddTransaction(ctx, Transaction{Amount: .01, Category: "food", Date: "2026-10-25"}); !errors.Is(err, ErrBudgetExceeded) {
			t.Fatal(err)
		}
	})
	t.Run("no_budget", func(t *testing.T) { clear(t); add(t, 10000) })
	t.Run("lowered_budget", func(t *testing.T) {
		clear(t)
		set(t, 500)
		add(t, 450)
		set(t, 400)
		if _, err := service.AddTransaction(ctx, Transaction{Amount: 1, Category: "food", Date: "2026-10-25"}); !errors.Is(err, ErrBudgetExceeded) {
			t.Fatal(err)
		}
	})
	t.Run("concurrent", func(t *testing.T) {
		clear(t)
		set(t, 100)
		var wg sync.WaitGroup
		results := make(chan error, 10)
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := service.AddTransaction(ctx, Transaction{Amount: 20, Category: "food", Date: "2026-10-25"})
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		ok, exceeded := 0, 0
		for err := range results {
			if err == nil {
				ok++
			} else if errors.Is(err, ErrBudgetExceeded) {
				exceeded++
			} else {
				t.Fatal(err)
			}
		}
		if ok != 5 || exceeded != 5 {
			t.Fatal(ok, exceeded)
		}
	})
	t.Run("restart_persists", func(t *testing.T) {
		clear(t)
		set(t, 500)
		saved := add(t, 100)
		if err := closeFn(); err != nil {
			t.Fatal(err)
		}
		closeFn = nil
		service, closeFn, err = Open(ctx)
		if err != nil {
			t.Fatal(err)
		}
		list, err := service.ListTransactions(ctx)
		if err != nil || len(list) != 1 || list[0] != saved {
			t.Fatal(list, err)
		}
	})
	t.Run("report_ttl_and_invalidation", func(t *testing.T) {
		clear(t)
		add(t, 100)
		from, to := "2026-10-01", "2026-10-31"
		first, err := service.GetReportSummary(ctx, from, to)
		if err != nil || first.Total != 100 {
			t.Fatal(first, err)
		}
		version, err := cache.Get(ctx, "report:version").Result()
		if err != nil {
			t.Fatal(err)
		}
		key := "report:summary:" + from + ":" + to + ":v" + version
		ttl, err := cache.TTL(ctx, key).Result()
		if err != nil || ttl <= 0 || ttl > 30*time.Second {
			t.Fatal(ttl, err)
		}
		if _, err := database.Exec("UPDATE expenses SET amount=150"); err != nil {
			t.Fatal(err)
		}
		hit, err := service.GetReportSummary(ctx, from, to)
		if err != nil || hit.Total != 100 {
			t.Fatal("cache hit expected", hit, err)
		}
		if err := cache.PExpire(ctx, key, time.Millisecond).Err(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
		fresh, err := service.GetReportSummary(ctx, from, to)
		if err != nil || fresh.Total != 150 {
			t.Fatal(fresh, err)
		}
		add(t, 25)
		fresh, err = service.GetReportSummary(ctx, from, to)
		if err != nil || fresh.Total != 175 {
			t.Fatal(fresh, err)
		}
	})
}
