package api

import (
	"context"
	"expense-bot/internal/store"
	"net/http"
	"time"

	"expense-bot/internal/ledger"
	"expense-bot/internal/payroll"
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
	NextPayday     *nextPaydayDTO        `json:"nextPayday"`
	Pace           *ledger.PaceInfo      `json:"pace"`
	Quarter        *quarterDTO           `json:"quarter"`
	Categories     []ledger.CategoryLine `json:"categories"`
}

type quarterDTO struct {
	Name     string  `json:"name"`
	Elapsed  float64 `json:"elapsed"`
	DaysLeft int     `json:"daysLeft"`
}

type nextPaydayDTO struct {
	Date     string `json:"date"`
	Kind     string `json:"kind"`
	DaysLeft int    `json:"daysLeft"`
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
		Categories:     ledger.BuildCategoryLines(m, categoryInfos(u.CategoryList())),
	}
	if resp.IsCurrent {
		days, per := ledger.DailyBudget(resp.Remaining, now)
		mode, _, _ := payroll.Schedule(u, s.cfg.Salary)
		pace, err := s.pace(ctx, userID, month, mode == payroll.ModeSplit, now, carry, m)
		if err != nil {
			return summaryResponse{}, err
		}
		resp.Pace = pace
		qn, qe, ql := ledger.QuarterProgress(now)
		resp.Quarter = &quarterDTO{Name: qn, Elapsed: qe, DaysLeft: ql}
		if mode == payroll.ModeSplit {
			// Budget until the next payday (007 FR-010).
			payday, kind := payroll.NextPayday(now, mode)
			days = payroll.BudgetDays(now, payday)
			per = resp.Remaining / int64(days)
			resp.NextPayday = &nextPaydayDTO{Date: payday.Format("2006-01-02"), Kind: string(kind), DaysLeft: days}
		}
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

// pace builds the home screen's spending-pace bar (008 FR-004). In split mode it reads the
// month's records to find the current pay period's start balance, income and expenses.
func (s *Server) pace(ctx context.Context, userID int64, month string, split bool, now time.Time, carry int64, m ledger.Month) (*ledger.PaceInfo, error) {
	if !split {
		return ledger.Pace(false, now, carry, m.Income, m.Expense, nil), nil
	}
	var txs []ledger.PaceTx
	f := store.HistoryFilter{Month: month, Limit: 100}
	for {
		page, next, err := s.store.QueryTransactions(ctx, userID, f)
		if err != nil {
			return nil, err
		}
		for _, t := range page {
			txs = append(txs, ledger.PaceTx{At: t.When(), Kind: t.Kind, Amount: t.Amount})
		}
		if next == "" {
			break
		}
		f.Cursor = next
	}
	return ledger.Pace(true, now, carry, m.Income, m.Expense, txs), nil
}
