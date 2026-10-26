package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"homework-7/ledger"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func call(t *testing.T, h http.Handler, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	if w.Code != status {
		t.Fatalf("%s %s = %d, want %d: %s", method, path, w.Code, status, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatal("wrong content type")
	}
	if !json.Valid(w.Body.Bytes()) {
		t.Fatal("invalid JSON response")
	}
	if status >= 400 {
		var v map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || len(v) != 1 || v["error"] == "" {
			t.Fatal("invalid error response")
		}
	}
	return w
}
func TestHTTPValidation(t *testing.T) {
	h := NewHandler()
	for _, tt := range []struct {
		name, method, path, body string
		status                   int
	}{
		{"bad_json", "POST", "/api/transactions", "{", 400},
		{"invalid_transaction", "POST", "/api/transactions", `{"amount":0}`, 400},
		{"invalid_budget", "POST", "/api/budgets", `{"category":"food","limit":0}`, 400},
		{"bad_date_range", "GET", "/api/reports/summary?from=bad&to=bad", "", 400},
		{"unknown_route", "GET", "/", "", 404},
		{"wrong_method", "DELETE", "/api/budgets", "", 405},
		{"report_method", "POST", "/api/reports/summary", "", 405},
	} {
		t.Run(tt.name, func(t *testing.T) { call(t, h, tt.method, tt.path, tt.body, tt.status) })
	}
}
func TestDatabaseUnavailable(t *testing.T) {
	h := NewHandler()
	for _, path := range []string{"/api/budgets", "/api/transactions", "/api/reports/summary?from=2026-01-01&to=2026-12-31"} {
		t.Run(path, func(t *testing.T) { call(t, h, "GET", path, "", 500) })
	}
}

func TestHTTPPostgresRedis(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a migrated *_test database for API integration tests")
	}
	t.Setenv("DATABASE_URL", dsn)
	index := os.Getenv("TEST_REDIS_DB")
	if index == "" {
		index = "14"
	}
	t.Setenv("REDIS_DB", index)
	ctx := context.Background()
	if err := ledger.Init(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ledger.Close() })
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err := db.QueryRow("SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("tests require a disposable *_test database")
	}
	clear := func() {
		if _, err := db.Exec("TRUNCATE expenses,budgets RESTART IDENTITY"); err != nil {
			t.Error(err)
		}
	}
	clear()
	defer clear()
	h := NewHandler()
	t.Run("empty", func(t *testing.T) {
		for _, path := range []string{"/api/budgets", "/api/transactions"} {
			if got := strings.TrimSpace(call(t, h, "GET", path, "", 200).Body.String()); got != "[]" {
				t.Fatal(got)
			}
		}
	})
	t.Run("budget", func(t *testing.T) {
		call(t, h, "POST", "/api/budgets", `{"category":"food","limit":500}`, 201)
		call(t, h, "GET", "/api/budgets", "", 200)
	})
	t.Run("transaction", func(t *testing.T) {
		w := call(t, h, "POST", "/api/transactions", `{"amount":450,"category":"food","description":"lunch","date":"2026-10-25"}`, 201)
		var value TransactionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil || value.ID != 1 || value.Amount != 450 || value.Category != "food" || value.Date != "2026-10-25" {
			t.Fatal(value, err)
		}
	})
	t.Run("exceeded", func(t *testing.T) {
		w := call(t, h, "POST", "/api/transactions", `{"amount":51,"category":"food","date":"2026-10-25"}`, 409)
		if strings.TrimSpace(w.Body.String()) != `{"error":"budget exceeded"}` {
			t.Fatal(w.Body.String())
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM expenses").Scan(&count); err != nil || count != 1 {
			t.Fatal(count, err)
		}
	})
	t.Run("report_twice", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			w := call(t, h, "GET", "/api/reports/summary?from=2026-10-01&to=2026-10-31", "", 200)
			var result ledger.ReportSummary
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Total != 450 {
				t.Fatal(result, err)
			}
		}
	})
	t.Run("restart", func(t *testing.T) {
		if err := ledger.Close(); err != nil {
			t.Fatal(err)
		}
		if err := ledger.Init(ctx); err != nil {
			t.Fatal(err)
		}
		w := call(t, h, "GET", "/api/transactions", "", 200)
		var values []TransactionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &values); err != nil || len(values) != 1 || values[0].ID != 1 {
			t.Fatal(values, err)
		}
		call(t, h, "GET", "/api/budgets", "", 200)
	})
}
