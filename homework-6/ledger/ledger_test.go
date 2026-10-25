package ledger

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTransactionValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		amount         int
		category, date string
		wantErr        bool
	}{
		{"valid", 100, "еда", "2026-10-25", false},
		{"zero_amount", 0, "еда", "2026-10-25", true},
		{"negative_amount", -1, "еда", "2026-10-25", true},
		{"empty_category", 100, "", "2026-10-25", true},
		{"blank_category", 100, " \t", "2026-10-25", true},
		{"invalid_date", 100, "еда", "2026-02-30", true},
		{"wrong_date_format", 100, "еда", "25.10.2026", true},
		{"empty_date", 100, "еда", "", true},
		{"leap_day", 100, "еда", "2024-02-29", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tx := Transaction{Amount: tt.amount, Category: tt.category, Date: tt.date}
			if err := tx.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBudgetValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, category string
		limit          int
		wantErr        bool
	}{
		{"valid", "еда", 5000, false}, {"zero_limit", "еда", 0, true}, {"negative_limit", "еда", -1, true}, {"empty_category", "", 100, true}, {"blank_category", " \n", 100, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := (Budget{Category: tt.category, Limit: tt.limit}).Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func isolate(t *testing.T) { t.Helper(); Reset(); t.Cleanup(Reset) }
func add(t *testing.T, amount int, category string) Transaction {
	t.Helper()
	tx, err := AddTransaction(Transaction{Amount: amount, Category: category, Date: "2026-10-25"})
	if err != nil {
		t.Fatal(err)
	}
	return tx
}
func set(t *testing.T, limit int) {
	t.Helper()
	if err := SetBudget(Budget{Category: "еда", Limit: limit}); err != nil {
		t.Fatal(err)
	}
}

func TestAddTransaction(t *testing.T) {
	tests := []struct {
		name         string
		first, next  int
		wantExceeded bool
	}{
		{"within_limit", 1000, 2000, false}, {"exact_limit", 3000, 2000, false}, {"exceeded", 3000, 2001, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			set(t, 5000)
			first := add(t, tt.first, "еда")
			before := ListTransactions()
			tx, err := AddTransaction(Transaction{Amount: tt.next, Category: "еда", Date: "2026-10-25"})
			if errors.Is(err, ErrBudgetExceeded) != tt.wantExceeded {
				t.Fatalf("AddTransaction() = %v", err)
			}
			if tt.wantExceeded {
				if !reflect.DeepEqual(before, ListTransactions()) {
					t.Fatal("rejected transaction changed storage")
				}
				return
			}
			if err != nil || tx.ID != 2 || first.ID != 1 {
				t.Fatalf("unexpected result %+v, %v", tx, err)
			}
			if len(ListTransactions()) != 2 {
				t.Fatal("transaction not saved")
			}
		})
	}
	t.Run("without_budget", func(t *testing.T) {
		isolate(t)
		add(t, 10000, "связь")
		if len(ListTransactions()) != 1 {
			t.Fatal("transaction missing")
		}
	})
	t.Run("invalid_not_saved", func(t *testing.T) {
		isolate(t)
		if _, err := AddTransaction(Transaction{}); err == nil {
			t.Fatal("expected validation error")
		}
		if len(ListTransactions()) != 0 {
			t.Fatal("invalid transaction saved")
		}
	})
	t.Run("budget_reduced_below_spent", func(t *testing.T) {
		isolate(t)
		set(t, 5000)
		add(t, 3000, "еда")
		set(t, 2000)
		if _, err := AddTransaction(Transaction{Amount: 1, Category: "еда", Date: "2026-10-25"}); !errors.Is(err, ErrBudgetExceeded) {
			t.Fatal(err)
		}
		if len(ListTransactions()) != 1 {
			t.Fatal("rejected transaction saved")
		}
	})
	t.Run("categories_independent", func(t *testing.T) { isolate(t); set(t, 5000); add(t, 5000, "связь"); add(t, 5000, "еда") })
}

func TestSetBudgetAndLists(t *testing.T) {
	isolate(t)
	if ListTransactions() == nil || ListBudgets() == nil {
		t.Fatal("empty lists must not be nil")
	}
	set(t, 5000)
	set(t, 6000)
	if err := SetBudget(Budget{Category: "еда", Limit: 0}); err == nil {
		t.Fatal("expected error")
	}
	if got := ListBudgets(); len(got) != 1 || got[0].Limit != 6000 {
		t.Fatalf("unexpected budgets: %+v", got)
	}
	if err := SetBudget(Budget{Category: "авто", Limit: 100}); err != nil {
		t.Fatal(err)
	}
	list := ListBudgets()
	if list[0].Category != "авто" {
		t.Fatal("budgets must be sorted")
	}
	list[0].Limit = 1
	if ListBudgets()[0].Limit != 100 {
		t.Fatal("returned budgets share mutable storage")
	}
	add(t, 100, "еда")
	txs := ListTransactions()
	txs[0].Amount = 999
	if ListTransactions()[0].Amount != 100 {
		t.Fatal("returned transactions share mutable storage")
	}
	Reset()
	if len(ListTransactions()) != 0 || len(ListBudgets()) != 0 {
		t.Fatal("Reset did not clear storage")
	}
	if add(t, 1, "еда").ID != 1 {
		t.Fatal("Reset did not reset IDs")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func TestLoadBudgets(t *testing.T) {
	tests := []struct {
		name, input string
		wantErr     bool
		count       int
	}{
		{"valid", `[{"category":"еда","limit":5000}]`, false, 1},
		{"empty", `[]`, false, 0}, {"bad_json", `{`, true, 0}, {"null", `null`, true, 0},
		{"invalid_atomic", `[{"category":"еда","limit":5000},{"category":"связь","limit":0}]`, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			err := LoadBudgets(strings.NewReader(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadBudgets() = %v", err)
			}
			if len(ListBudgets()) != tt.count {
				t.Fatal("unexpected saved budgets")
			}
		})
	}
	t.Run("read_error", func(t *testing.T) {
		isolate(t)
		if err := LoadBudgets(failingReader{}); err == nil {
			t.Fatal("expected read error")
		}
	})
}
