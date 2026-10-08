package api

import (
	"context"
	"net/http"

	"expense-bot/internal/ledger"
)

type summaryResponse struct {
	Month          string                `json:"month"`
	IsCurrent      bool                  `json:"isCurrent"`
	Income         int64                 `json:"income"`
	Expense        int64                 `json:"expense"`
	SavingsNet     int64                 `json:"savingsNet"`
	CreditCharged  int64                 `json:"creditCharged"`
	CreditRepaid   int64                 `json:"creditRepaid"`
	CarryOver      int64                 `json:"carryOver"`
	Remaining      int64                 `json:"remaining"`
	SavingsBalance int64                 `json:"savingsBalance"`
	CreditDebt     int64                 `json:"creditDebt"`
	DaysLeft       *int                  `json:"daysLeft"`
	DailyBudget    *int64                `json:"dailyBudget"`
	Categories     []ledger.CategoryLine `json:"categories"`
}

// buildSummary computes the month report (FR-011, FR-012).
func (s *Server) buildSummary(ctx context.Context, userID int64, month string) (summaryResponse, error) {
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return summaryResponse{}, err
	}
	m, err := s.store.GetMonth(ctx, userID, month)
	if err != nil {
		return summaryResponse{}, err
	}
	prior, err := s.store.MonthsBefore(ctx, userID, month)
	if err != nil {
		return summaryResponse{}, err
	}
	carry := ledger.CarryOver(prior)
	now := ledger.Now()
	resp := summaryResponse{
		Month:          month,
		IsCurrent:      month == ledger.MonthKey(now),
		Income:         m.Income,
		Expense:        m.Expense,
		SavingsNet:     m.SavingsNet,
		CreditCharged:  m.CreditCharged,
		CreditRepaid:   m.CreditRepaid,
		CarryOver:      carry,
		Remaining:      carry + m.CashNet,
		SavingsBalance: u.SavingsBalance,
		CreditDebt:     u.CreditDebt,
		Categories:     ledger.BuildCategoryLines(m, s.effective(u).Limits),
	}
	if resp.IsCurrent {
		days, per := ledger.DailyBudget(resp.Remaining, now)
		resp.DaysLeft, resp.DailyBudget = &days, &per
	}
	return resp, nil
}

// parseReportMonth validates ?month= (default current, never in the future).
func parseReportMonth(r *http.Request, required bool) (string, string) {
	cur := ledger.MonthKey(ledger.Now())
	month := r.URL.Query().Get("month")
	if month == "" {
		if required {
			return "", "Укажите месяц"
		}
		return cur, ""
	}
	if _, err := ledger.ParseMonth(month); err != nil {
		return "", err.Error()
	}
	if month > cur {
		return "", "Месяц ещё не наступил"
	}
	return month, ""
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	month, msg := parseReportMonth(r, false)
	if msg != "" {
		validation(w, "month", msg)
		return
	}
	resp, err := s.buildSummary(r.Context(), userFrom(r.Context()).ID, month)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
