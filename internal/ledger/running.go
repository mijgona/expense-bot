package ledger

import (
	"sort"
	"time"
)

// Event is one change of a running balance (savings or card debt).
// At is the business time (occurredAt); Seq breaks ties within the same At (createdAt).
type Event struct {
	At    time.Time
	Seq   time.Time
	Delta int64
}

// Violation is the first point where a change makes a balance newly or more negative.
type Violation struct {
	At      time.Time
	Balance int64
}

// SavingsDelta returns the savings-balance effect of a kind (deposit +, withdrawal −).
func SavingsDelta(k Kind, amount int64) (int64, bool) {
	switch k {
	case KindSavingsDeposit:
		return amount, true
	case KindSavingsWithdrawal:
		return -amount, true
	}
	return 0, false
}

// DebtDelta returns the card-debt effect of a kind (purchase +, repayment −).
func DebtDelta(k Kind, amount int64) (int64, bool) {
	switch k {
	case KindCreditPurchase:
		return amount, true
	case KindCreditRepayment:
		return -amount, true
	}
	return 0, false
}

func eventLess(a, b Event) bool {
	if !a.At.Equal(b.At) {
		return a.At.Before(b.At)
	}
	return a.Seq.Before(b.Seq)
}

// CheckRunning replays both event lists chronologically and returns the first instant
// where the balance after the change is below zero AND below the balance before it
// (constitution II / research R3). History that was already negative is allowed as long
// as the change does not make it worse at any instant.
func CheckRunning(before, after []Event) *Violation {
	b := sortedCopy(before)
	a := sortedCopy(after)

	instants := make([]Event, 0, len(a)+len(b))
	instants = append(instants, a...)
	instants = append(instants, b...)
	sort.SliceStable(instants, func(i, j int) bool { return eventLess(instants[i], instants[j]) })

	var balB, balA int64
	ib, ia := 0, 0
	for _, t := range instants {
		for ib < len(b) && !eventLess(t, b[ib]) {
			balB += b[ib].Delta
			ib++
		}
		for ia < len(a) && !eventLess(t, a[ia]) {
			balA += a[ia].Delta
			ia++
		}
		if balA < 0 && balA < balB {
			return &Violation{At: t.At, Balance: balA}
		}
	}
	return nil
}

func sortedCopy(evs []Event) []Event {
	out := append([]Event(nil), evs...)
	sort.SliceStable(out, func(i, j int) bool { return eventLess(out[i], out[j]) })
	return out
}
