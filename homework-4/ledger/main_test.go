package main

import (
	"errors"
	"strings"
	"testing"
)

func resetLedger(t *testing.T) {
	t.Helper()
	oldTransactions, oldBudgets := transactions, budgets
	transactions = make([]Transaction, 0)
	budgets = make(map[string]Budget)
	t.Cleanup(func() { transactions, budgets = oldTransactions, oldBudgets })
}

func TestCheckValid(t *testing.T) {
	cases := []struct {
		name  string
		value Validatable
		valid bool
	}{
		{"transaction", Transaction{Amount: 1, Category: "Еда", Date: "2026-10-25"}, true},
		{"zero amount", Transaction{Amount: 0, Category: "Еда", Date: "2026-10-25"}, false},
		{"negative amount", Transaction{Amount: -1, Category: "Еда", Date: "2026-10-25"}, false},
		{"empty category", Transaction{Amount: 1, Date: "2026-10-25"}, false},
		{"blank category", Transaction{Amount: 1, Category: " \t", Date: "2026-10-25"}, false},
		{"invalid date", Transaction{Amount: 1, Category: "Еда", Date: "2026-02-30"}, false},
		{"empty date", Transaction{Amount: 1, Category: "Еда"}, false},
		{"budget", Budget{Category: "Еда", Limit: 1}, true},
		{"zero limit", Budget{Category: "Еда", Limit: 0}, false},
		{"negative limit", Budget{Category: "Еда", Limit: -1}, false},
		{"empty budget category", Budget{Limit: 1}, false},
		{"blank budget category", Budget{Category: " \t", Limit: 1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := CheckValid(tc.value); (err == nil) != tc.valid {
				t.Fatalf("CheckValid() = %v, valid = %v", err, tc.valid)
			}
		})
	}
}

func TestLedger(t *testing.T) {
	resetLedger(t)
	if err := SetBudget(Budget{Category: "Еда", Limit: 100}); err != nil {
		t.Fatal(err)
	}
	if err := SetBudget(Budget{Category: "Еда", Limit: 0}); err == nil {
		t.Fatal("invalid budget accepted")
	}
	if budgets["Еда"].Limit != 100 {
		t.Fatal("invalid update changed budget")
	}
	tx := Transaction{Amount: 60, Category: "Еда", Date: "2026-10-25"}
	if err := AddTransaction(tx); err != nil {
		t.Fatal(err)
	}
	tx.Amount = 41
	if err := AddTransaction(tx); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("expected budget error: %v", err)
	}
	tx.Amount = 40
	if err := AddTransaction(tx); err != nil {
		t.Fatal(err)
	}
	tx.Category = "Связь"
	if err := AddTransaction(tx); err != nil {
		t.Fatal(err)
	}
	tx.Category = " "
	if err := AddTransaction(tx); err == nil {
		t.Fatal("invalid transaction accepted")
	}
	tx.Category, tx.Date = "Еда", "wrong"
	if err := AddTransaction(tx); err == nil || errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("validation must precede budget check: %v", err)
	}
	if len(transactions) != 3 || transactions[2].ID != 3 {
		t.Fatal("rejected transactions changed storage or IDs")
	}
	saved := ListTransactions()
	saved[0].Amount = 1
	if transactions[0].Amount != 60 {
		t.Fatal("ListTransactions exposed storage")
	}
	if err := SetBudget(Budget{Category: "Еда", Limit: 50}); err != nil {
		t.Fatal(err)
	}
	tx.Date, tx.Amount = "2026-10-25", 1
	if err := AddTransaction(tx); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("lowered limit not enforced: %v", err)
	}
	if err := SetBudget(Budget{Category: "Еда", Limit: 101}); err != nil {
		t.Fatal(err)
	}
	if err := AddTransaction(tx); err != nil {
		t.Fatal(err)
	}
}

func TestLoadBudgets(t *testing.T) {
	resetLedger(t)
	for _, input := range []string{"", "null", "{}", "[", `[{"category":"Еда","limit":100},{"category":"Связь","limit":0}]`} {
		if err := LoadBudgets(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted %q", input)
		}
		if len(budgets) != 0 {
			t.Fatal("partially loaded invalid budgets")
		}
	}
	if err := LoadBudgets(strings.NewReader(`[]`)); err != nil {
		t.Fatal(err)
	}
	if err := LoadBudgets(strings.NewReader(`[{"category":"Еда","limit":100},{"category":"Еда","limit":200}]`)); err != nil {
		t.Fatal(err)
	}
	if budgets["Еда"].Limit != 200 {
		t.Fatal("last duplicate must win")
	}
	if err := LoadBudgets(strings.NewReader(`[{"category":"Связь","limit":50}]`)); err != nil {
		t.Fatal(err)
	}
	if len(budgets) != 2 {
		t.Fatal("existing budget lost")
	}
}
