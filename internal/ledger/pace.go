package ledger

import (
	"fmt"
	"time"
)

// PaceTx is one record of the current month, as the pace calculation needs it.
type PaceTx struct {
	At     time.Time
	Kind   Kind
	Amount int64
}

// PaceInfo is the home screen's spending-pace bar (feature 008 FR-004).
type PaceInfo struct {
	Spent   int64   `json:"spent"`
	Budget  int64   `json:"budget"`
	Elapsed float64 `json:"elapsed"`
	Period  string  `json:"period"` // month | advance | rest
}

// advanceDay mirrors payroll.AdvanceDay (ledger cannot import payroll).
const advanceDay = 15

func dayStart(t time.Time) time.Time {
	t = t.In(Location)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Location)
}

func daysBetween(a, b time.Time) int { return int(dayStart(b).Sub(dayStart(a)).Hours()/24 + 0.5) }

// Pace computes spent vs available money for the current budget period. Spent is expenses only;
// everything else that moves cash (savings, card repayments) changes the budget instead, so
// Budget − Spent always equals the summary's remaining. With one payment the period is the month
// (budget = carryOver + monthCashNet + monthExpense). With two payments it is the 1st–14th or the
// 15th–last day: budget = carryOver + the cashNet of records before the period + the non-expense
// cashNet inside it. Returns nil when the budget is not positive.
func Pace(split bool, now time.Time, carryOver, monthCashNet, monthExpense int64, monthTxs []PaceTx) *PaceInfo {
	today := dayStart(now)
	first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, Location)
	last := first.AddDate(0, 1, -1)

	if !split {
		p := &PaceInfo{Spent: monthExpense, Budget: carryOver + monthCashNet + monthExpense, Period: "month",
			Elapsed: float64(today.Day()) / float64(last.Day())}
		if p.Budget <= 0 {
			return nil
		}
		return p
	}

	start, end, period := first, first.AddDate(0, 0, advanceDay-2), "advance"
	if today.Day() >= advanceDay {
		start, end, period = first.AddDate(0, 0, advanceDay-1), last, "rest"
	}
	p := &PaceInfo{Period: period, Budget: carryOver,
		Elapsed: float64(daysBetween(start, today)+1) / float64(daysBetween(start, end)+1)}
	for _, t := range monthTxs {
		switch {
		case dayStart(t.At).Before(start):
			p.Budget += Apply(t.Kind, "", t.Amount).Month.CashNet
		case t.Kind == KindExpense:
			p.Spent += t.Amount
		default:
			p.Budget += Apply(t.Kind, "", t.Amount).Month.CashNet
		}
	}
	if p.Budget <= 0 {
		return nil
	}
	return p
}

// QuarterBounds returns the calendar quarter containing now (Dushanbe): "Q4 2026", first and last day.
func QuarterBounds(now time.Time) (name string, start, end time.Time) {
	today := dayStart(now)
	q := (int(today.Month())-1)/3 + 1
	start = time.Date(today.Year(), time.Month((q-1)*3+1), 1, 0, 0, 0, 0, Location)
	end = start.AddDate(0, 3, -1)
	return fmt.Sprintf("Q%d %d", q, today.Year()), start, end
}

// QuarterProgress returns the quarter's name, the share elapsed (today included) and the days
// left (today included).
func QuarterProgress(now time.Time) (name string, elapsed float64, daysLeft int) {
	name, start, end := QuarterBounds(now)
	length := daysBetween(start, end) + 1
	passed := daysBetween(start, now) + 1
	return name, float64(passed) / float64(length), daysBetween(now, end) + 1
}
