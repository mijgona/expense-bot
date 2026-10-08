// Package ledger holds the pure money rules: transaction kinds, how each kind moves
// month aggregates and user balances, and report math. No I/O.
package ledger

import (
	"fmt"
	"sort"
	"time"

	"expense-bot/internal/category"
)

// Kind is a ledger transaction type. Amounts are always positive; Kind gives direction.
type Kind string

const (
	KindExpense           Kind = "expense"
	KindIncome            Kind = "income"
	KindSavingsDeposit    Kind = "savings_deposit"
	KindSavingsWithdrawal Kind = "savings_withdrawal"
	KindCreditPurchase    Kind = "credit_purchase"
	KindCreditRepayment   Kind = "credit_repayment"
)

// ParseKind validates a kind string.
func ParseKind(s string) (Kind, error) {
	switch k := Kind(s); k {
	case KindExpense, KindIncome, KindSavingsDeposit, KindSavingsWithdrawal, KindCreditPurchase, KindCreditRepayment:
		return k, nil
	}
	return "", fmt.Errorf("неизвестный тип записи %q", s)
}

// NeedsCategory reports whether the kind requires a category.
func (k Kind) NeedsCategory() bool { return k == KindExpense || k == KindCreditPurchase }

// Month is a month aggregate (all values in diram). It is also used as a delta.
type Month struct {
	Month            string           `firestore:"month"`
	Income           int64            `firestore:"income"`
	Expense          int64            `firestore:"expense"`
	ByCategory       map[string]int64 `firestore:"byCategory"`
	SavingsNet       int64            `firestore:"savingsNet"`
	CreditCharged    int64            `firestore:"creditCharged"`
	ByCreditCategory map[string]int64 `firestore:"byCreditCategory"`
	CreditRepaid     int64            `firestore:"creditRepaid"`
	CashNet          int64            `firestore:"cashNet"`
}

// Delta is the effect of one transaction.
type Delta struct {
	Month          Month
	SavingsBalance int64
	CreditDebt     int64
}

// Apply returns how a transaction of kind/category/amount changes the aggregates (research R4).
// cashNet = income − expense − savingsNet − creditRepaid.
func Apply(kind Kind, cat string, amount int64) Delta {
	var d Delta
	m := &d.Month
	switch kind {
	case KindIncome:
		m.Income = amount
		m.CashNet = amount
	case KindExpense:
		m.Expense = amount
		m.ByCategory = map[string]int64{cat: amount}
		m.CashNet = -amount
	case KindSavingsDeposit:
		m.SavingsNet = amount
		m.CashNet = -amount
		d.SavingsBalance = amount
	case KindSavingsWithdrawal:
		m.SavingsNet = -amount
		m.CashNet = amount
		d.SavingsBalance = -amount
	case KindCreditPurchase:
		m.CreditCharged = amount
		m.ByCreditCategory = map[string]int64{cat: amount}
		d.CreditDebt = amount
	case KindCreditRepayment:
		m.CreditRepaid = amount
		m.CashNet = -amount
		d.CreditDebt = -amount
	}
	return d
}

// Add accumulates d into m (used when rebuilding aggregates).
func (m *Month) Add(d Month) {
	m.Income += d.Income
	m.Expense += d.Expense
	m.SavingsNet += d.SavingsNet
	m.CreditCharged += d.CreditCharged
	m.CreditRepaid += d.CreditRepaid
	m.CashNet += d.CashNet
	for k, v := range d.ByCategory {
		if m.ByCategory == nil {
			m.ByCategory = map[string]int64{}
		}
		m.ByCategory[k] += v
	}
	for k, v := range d.ByCreditCategory {
		if m.ByCreditCategory == nil {
			m.ByCreditCategory = map[string]int64{}
		}
		m.ByCreditCategory[k] += v
	}
}

// CarryOver sums cashNet over all months before the report month.
func CarryOver(prior []Month) int64 {
	var total int64
	for _, m := range prior {
		total += m.CashNet
	}
	return total
}

// CategoryLine is one row of the per-category report.
type CategoryLine struct {
	Name   string `json:"name"`
	Label  string `json:"label"`
	Spent  int64  `json:"spent"`
	Limit  *int64 `json:"limit"`
	Status string `json:"status"` // ok | warn | over | none
}

// BuildCategoryLines returns every configured category (spent desc), then legacy names.
func BuildCategoryLines(m Month) []CategoryLine {
	var lines []CategoryLine
	known := map[string]bool{}
	for _, c := range category.All() {
		known[c.Name] = true
		limit := int64(c.Limit) * PerSomoni
		spent := m.ByCategory[c.Name]
		lines = append(lines, CategoryLine{Name: c.Name, Label: c.Label, Spent: spent, Limit: &limit, Status: limitStatus(spent, limit)})
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Spent > lines[j].Spent })

	var legacy []CategoryLine
	for name, spent := range m.ByCategory {
		if !known[name] && spent != 0 {
			legacy = append(legacy, CategoryLine{Name: name, Label: name, Spent: spent, Status: "none"})
		}
	}
	sort.Slice(legacy, func(i, j int) bool { return legacy[i].Spent > legacy[j].Spent })
	return append(lines, legacy...)
}

func limitStatus(spent, limit int64) string {
	switch {
	case limit <= 0:
		return "none"
	case spent > limit:
		return "over"
	case spent*100 > limit*80:
		return "warn"
	default:
		return "ok"
	}
}

// DailyBudget splits remaining across the days left in now's month, today included.
func DailyBudget(remaining int64, now time.Time) (daysLeft int, perDay int64) {
	now = now.In(Location)
	daysLeft = DaysInMonth(now) - now.Day() + 1
	return daysLeft, remaining / int64(daysLeft)
}
