package ledger

import (
	"context"
	"math"
	"testing"
)

func TestTransactionValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		amount         float64
		category, date string
		bad            bool
	}{
		{"valid", 450, "food", "2026-10-25", false}, {"cents", 450.25, "food", "2026-10-25", false},
		{"zero", 0, "food", "2026-10-25", true}, {"negative", -1, "food", "2026-10-25", true},
		{"category", 1, " ", "2026-10-25", true}, {"date", 1, "food", "2026-02-30", true},
		{"precision", 1.001, "food", "2026-10-25", true}, {"tiny_precision", 1.000001, "food", "2026-10-25", true},
		{"too_large", 1e12, "food", "2026-10-25", true}, {"nan", math.NaN(), "food", "2026-10-25", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := (Transaction{Amount: tt.amount, Category: tt.category, Date: tt.date}).Validate()
			if (err != nil) != tt.bad {
				t.Fatal(err)
			}
		})
	}
}
func TestBudgetValidate(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		budget Budget
		bad    bool
	}{{"valid", Budget{"food", 5000}, false}, {"cents", Budget{"food", 0.01}, false}, {"zero", Budget{"food", 0}, true}, {"negative", Budget{"food", -1}, true}, {"category", Budget{"", 100}, true}, {"precision", Budget{"food", 1.234}, true}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := CheckValid(tt.budget); (err != nil) != tt.bad {
				t.Fatal(err)
			}
		})
	}
}
func TestReportRange(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		from, to string
		bad      bool
	}{{"2026-01-01", "2026-12-31", false}, {"2026-01-01", "2026-01-01", false}, {"2026-02-30", "2026-12-31", true}, {"2026-01-01", "", true}, {"2026-12-31", "2026-01-01", true}} {
		if err := ValidateReportRange(tt.from, tt.to); (err != nil) != tt.bad {
			t.Fatal(err)
		}
	}
}
func TestNotInitialized(t *testing.T) {
	if database != nil {
		t.Skip("database initialized")
	}
	if _, err := ListBudgets(); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ListTransactions(); err == nil {
		t.Fatal("expected error")
	}
	if err := SetBudget(Budget{"food", 500}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := AddTransaction(Transaction{Amount: 1, Category: "food", Date: "2026-10-25"}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := GetReportSummary(context.Background(), "2026-01-01", "2026-12-31"); err == nil {
		t.Fatal("expected error")
	}
}
