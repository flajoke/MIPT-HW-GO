package api

import "homework-7/ledger"

type CreateTransactionRequest struct {
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Date        string  `json:"date"`
}

type TransactionResponse struct {
	ID          int     `json:"id"`
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Date        string  `json:"date"`
}

type CreateBudgetRequest struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

type BudgetResponse struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

func (r CreateTransactionRequest) toDomain() ledger.Transaction {
	return ledger.Transaction{Amount: r.Amount, Category: r.Category, Description: r.Description, Date: r.Date}
}
func transactionResponse(tx ledger.Transaction) TransactionResponse {
	return TransactionResponse{ID: tx.ID, Amount: tx.Amount, Category: tx.Category, Description: tx.Description, Date: tx.Date}
}
func (r CreateBudgetRequest) toDomain() ledger.Budget {
	return ledger.Budget{Category: r.Category, Limit: r.Limit}
}
func budgetResponse(b ledger.Budget) BudgetResponse {
	return BudgetResponse{Category: b.Category, Limit: b.Limit}
}
