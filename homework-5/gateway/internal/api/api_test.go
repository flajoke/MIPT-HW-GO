package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestAPI(t *testing.T) {
	h := NewHandler()
	call := func(method, path, body string, status int) string {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		if w.Code != status {
			t.Fatalf("%s %s: got %d %s, want %d", method, path, w.Code, w.Body.String(), status)
		}
		if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatal("missing JSON content type")
		}
		if !json.Valid(w.Body.Bytes()) {
			t.Fatal("invalid JSON response")
		}
		return strings.TrimSpace(w.Body.String())
	}
	for _, path := range []string{"/api/transactions", "/api/budgets"} {
		if got := call("GET", path, "", 200); got != "[]" {
			t.Fatalf("empty list: %s", got)
		}
	}
	call("POST", "/api/budgets", `{"category":"food","limit":100}`, 201)
	call("POST", "/api/budgets", `{"category":"food","limit":0}`, 400)
	call("POST", "/api/transactions", `{"amount":60,"category":"food","date":"2026-10-25"}`, 201)
	got := call("POST", "/api/transactions", `{"amount":41,"category":"food","date":"2026-10-25"}`, 409)
	if got != `{"error":"budget exceeded"}` {
		t.Fatal(got)
	}
	call("POST", "/api/transactions", `{"amount":40,"category":"food","date":"2026-10-25"}`, 201)
	for _, body := range []string{`{`, `null`, `[]`, `{} {}`, `{"amount":0}`, `{"amount":1,"category":"food","date":"2026-02-30"}`, `{"amount":1,"category":" ","date":"2026-10-25"}`} {
		call("POST", "/api/transactions", body, 400)
	}
	call("POST", "/api/budgets", `{"category":"food","limit":120}`, 201)
	call("POST", "/api/transactions", `{"amount":20,"category":"food","date":"2026-10-25"}`, 201)
	var saved []TransactionResponse
	json.Unmarshal([]byte(call("GET", "/api/transactions", "", 200)), &saved)
	if len(saved) != 3 || saved[2].ID != 3 {
		t.Fatalf("unexpected transactions: %+v", saved)
	}
	call("DELETE", "/api/budgets", "", 405)
	call("GET", "/missing", "", 404)
	// Concurrent requests must receive unique IDs without losing transactions.
	var wg sync.WaitGroup
	ids := make(chan int, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/transactions", strings.NewReader(`{"amount":1,"category":"other","date":"2026-10-25"}`)))
			if w.Code != 201 {
				t.Error(w.Code)
				return
			}
			var tx TransactionResponse
			if err := json.Unmarshal(w.Body.Bytes(), &tx); err != nil {
				t.Error(err)
			}
			ids <- tx.ID
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[int]bool{}
	for id := range ids {
		if seen[id] {
			t.Error(fmt.Sprintf("duplicate ID %d", id))
		}
		seen[id] = true
	}
	if len(seen) != 20 {
		t.Fatal("lost transactions")
	}
}
