// Package payroll holds the salary schedule rules (feature 007): one payment on the last day
// of the month, or an advance on the 15th plus the rest on the last day. Pure date and amount
// logic plus the payday reminder loop (reminder.go).
package payroll

import (
	"fmt"
	"strings"
	"time"

	"expense-bot/internal/ledger"
)

// Mode is the salary schedule.
type Mode string

const (
	ModeSingle Mode = "single"
	ModeSplit  Mode = "split"
)

// ParseMode validates a mode ("" means single).
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case "", ModeSingle:
		return ModeSingle, nil
	case ModeSplit:
		return ModeSplit, nil
	}
	return "", fmt.Errorf("неизвестный режим зарплаты %q", s)
}

// Kind is one expected payment of a month.
type Kind string

const (
	KindAdvance Kind = "advance"
	KindRest    Kind = "rest"
	KindFull    Kind = "full"
)

// ParseKind validates a payment kind.
func ParseKind(s string) (Kind, error) {
	switch k := Kind(s); k {
	case KindAdvance, KindRest, KindFull:
		return k, nil
	}
	return "", fmt.Errorf("неизвестная выплата %q", s)
}

// AdvanceDay is the day of the month the advance is paid.
const AdvanceDay = 15

// Payment is one expected salary payment.
type Payment struct {
	Kind   Kind
	Amount int64 // diram
	Payday time.Time
	Note   string
}

// Note is the income note for a payment kind.
func Note(k Kind) string {
	switch k {
	case KindAdvance:
		return "Аванс"
	case KindRest:
		return "Зарплата (остаток)"
	default:
		return "Зарплата"
	}
}

// TxID is the deterministic ID of the income record for a payment (idempotent recording).
func TxID(month string, k Kind) string { return "p_" + month + "_" + string(k) }

// IsPayoutTxID reports whether a transaction ID is a payment record.
func IsPayoutTxID(id string) bool { return strings.HasPrefix(id, "p_") }

func monthStart(month string) time.Time {
	t, _ := ledger.ParseMonth(month)
	return t
}

// LastDay is the last calendar day of a "YYYY-MM" month (midnight, Dushanbe).
func LastDay(month string) time.Time {
	return monthStart(month).AddDate(0, 1, -1)
}

// EffectiveAdvance returns the stored advance if it is within 1 с. … salary − 1 с.,
// otherwise half the salary rounded down to whole somoni.
func EffectiveAdvance(salary int64, advance *int64) int64 {
	if advance != nil && *advance >= ledger.PerSomoni && *advance <= salary-ledger.PerSomoni {
		return *advance
	}
	return salary / 2 / ledger.PerSomoni * ledger.PerSomoni
}

// Expected returns the month's expected payments, in payday order.
func Expected(mode Mode, salary, advance int64, month string) []Payment {
	last := LastDay(month)
	if mode != ModeSplit {
		return []Payment{{Kind: KindFull, Amount: salary, Payday: last, Note: Note(KindFull)}}
	}
	start := monthStart(month)
	return []Payment{
		{Kind: KindAdvance, Amount: advance, Payday: start.AddDate(0, 0, AdvanceDay-1), Note: Note(KindAdvance)},
		{Kind: KindRest, Amount: salary - advance, Payday: last, Note: Note(KindRest)},
	}
}

func dayOf(t time.Time) time.Time {
	t = t.In(ledger.Location)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, ledger.Location)
}

// NextPayday is the first payday strictly after today (on a payday, the next one).
func NextPayday(now time.Time, mode Mode) (time.Time, Kind) {
	today := dayOf(now)
	month := ledger.MonthKey(today)
	for i := 0; i < 3; i++ {
		for _, p := range Expected(mode, 0, 0, month) {
			if p.Payday.After(today) {
				return p.Payday, p.Kind
			}
		}
		month = ledger.NextMonth(month)
	}
	return today, KindFull // unreachable
}

// BudgetDays is the number of days from today up to (not including) payday, at least 1.
func BudgetDays(now, payday time.Time) int {
	n := int(dayOf(payday).Sub(dayOf(now)).Hours()/24 + 0.5)
	if n < 1 {
		return 1
	}
	return n
}
