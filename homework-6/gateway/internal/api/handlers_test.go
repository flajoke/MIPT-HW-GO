package api

import (
	"encoding/json"
	"homework-6/ledger"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	ledger.Reset()
	t.Cleanup(ledger.Reset)
	return NewHandler()
}
func request(t *testing.T, h http.Handler, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s %s: status %d, want %d; body %s", method, path, w.Code, status, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if !json.Valid(w.Body.Bytes()) {
		t.Fatalf("invalid JSON response: %s", w.Body.String())
	}
	if status >= 400 {
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		msg, ok := body["error"].(string)
		if !ok || msg == "" || len(body) != 1 {
			t.Fatalf("invalid error response: %v", body)
		}
	}
	return w
}
func decodeResponse[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestBudgets(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		h := testHandler(t)
		w := request(t, h, "POST", "/api/budgets", `{"category":"еда","limit":5000}`, 201)
		want := BudgetResponse{Category: "еда", Limit: 5000}
		if got := decodeResponse[BudgetResponse](t, w); got != want {
			t.Fatalf("budget = %+v, want %+v", got, want)
		}
		list := decodeResponse[[]BudgetResponse](t, request(t, h, "GET", "/api/budgets", "", 200))
		if !reflect.DeepEqual(list, []BudgetResponse{want}) {
			t.Fatalf("budgets = %+v", list)
		}
	})
	t.Run("update", func(t *testing.T) {
		h := testHandler(t)
		request(t, h, "POST", "/api/budgets", `{"category":"еда","limit":5000}`, 201)
		request(t, h, "POST", "/api/budgets", `{"category":"еда","limit":6000}`, 201)
		list := decodeResponse[[]BudgetResponse](t, request(t, h, "GET", "/api/budgets", "", 200))
		if len(list) != 1 || list[0].Limit != 6000 {
			t.Fatalf("update = %+v", list)
		}
	})
	tests := []struct{ name, body string }{
		{"bad_json", `{`}, {"zero_limit", `{"category":"еда","limit":0}`}, {"negative_limit", `{"category":"еда","limit":-1}`}, {"empty_category", `{"category":"","limit":100}`}, {"unknown_field", `{"category":"еда","limit":100,"extra":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := testHandler(t)
			request(t, h, "POST", "/api/budgets", tt.body, 400)
			if len(ledger.ListBudgets()) != 0 {
				t.Fatal("invalid budget saved")
			}
		})
	}
}

func TestTransactions(t *testing.T) {
	const valid = `{"amount":3000,"category":"еда","description":"обед","date":"2026-10-25"}`
	setup := func(t *testing.T) http.Handler {
		t.Helper()
		h := testHandler(t)
		request(t, h, "POST", "/api/budgets", `{"category":"еда","limit":5000}`, 201)
		return h
	}
	t.Run("ok", func(t *testing.T) {
		h := setup(t)
		want := TransactionResponse{ID: 1, Amount: 3000, Category: "еда", Description: "обед", Date: "2026-10-25"}
		got := decodeResponse[TransactionResponse](t, request(t, h, "POST", "/api/transactions", valid, 201))
		if got != want {
			t.Fatalf("transaction = %+v, want %+v", got, want)
		}
		list := decodeResponse[[]TransactionResponse](t, request(t, h, "GET", "/api/transactions", "", 200))
		if !reflect.DeepEqual(list, []TransactionResponse{want}) {
			t.Fatalf("transactions = %+v", list)
		}
	})
	t.Run("exceeded", func(t *testing.T) {
		h := setup(t)
		request(t, h, "POST", "/api/transactions", valid, 201)
		before := ledger.ListTransactions()
		w := request(t, h, "POST", "/api/transactions", valid, 409)
		if got := decodeResponse[map[string]string](t, w); got["error"] != "budget exceeded" {
			t.Fatalf("error = %v", got)
		}
		if !reflect.DeepEqual(before, ledger.ListTransactions()) {
			t.Fatal("rejected transaction changed storage")
		}
	})
	t.Run("exact_limit", func(t *testing.T) {
		h := setup(t)
		request(t, h, "POST", "/api/transactions", valid, 201)
		request(t, h, "POST", "/api/transactions", `{"amount":2000,"category":"еда","date":"2026-10-25"}`, 201)
	})
	tests := []struct{ name, body string }{
		{"bad_json", `{`}, {"empty_body", ``}, {"multiple_objects", `{} {}`}, {"wrong_type", `{"amount":"100"}`}, {"null", `null`}, {"array", `[]`},
		{"zero_amount", `{"amount":0,"category":"еда","date":"2026-10-25"}`},
		{"negative_amount", `{"amount":-1,"category":"еда","date":"2026-10-25"}`},
		{"empty_category", `{"amount":1,"category":"","date":"2026-10-25"}`},
		{"invalid_date", `{"amount":1,"category":"еда","date":"2026-02-30"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := setup(t)
			request(t, h, "POST", "/api/transactions", tt.body, 400)
			if len(ledger.ListTransactions()) != 0 {
				t.Fatal("invalid transaction saved")
			}
		})
	}
}

func TestEmptyListsAndRoutes(t *testing.T) {
	for _, path := range []string{"/api/budgets", "/api/transactions"} {
		t.Run(path, func(t *testing.T) {
			h := testHandler(t)
			w := request(t, h, "GET", path, "", 200)
			if strings.TrimSpace(w.Body.String()) != "[]" {
				t.Fatalf("empty list: %s", w.Body.String())
			}
			w = request(t, h, "DELETE", path, "", 405)
			if w.Header().Get("Allow") != "GET, POST" {
				t.Fatal("missing Allow header")
			}
		})
	}
	t.Run("not_found", func(t *testing.T) { request(t, testHandler(t), "GET", "/missing", "", 404) })
}
