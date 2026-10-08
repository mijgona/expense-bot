package ledger

import (
	"testing"
	"time"
)

func day(d, m int) time.Time { return time.Date(2026, time.Month(m), d, 12, 0, 0, 0, Location) }

func ev(at time.Time, delta int64) Event { return Event{At: at, Seq: at, Delta: delta} }

func TestCheckRunningEmpty(t *testing.T) {
	if v := CheckRunning(nil, nil); v != nil {
		t.Errorf("empty: %+v", v)
	}
}

func TestCheckRunningLowerDeposit(t *testing.T) {
	before := []Event{ev(day(1, 6), 100000), ev(day(10, 6), -80000)}
	after := []Event{ev(day(1, 6), 50000), ev(day(10, 6), -80000)}
	v := CheckRunning(before, after)
	if v == nil || !v.At.Equal(day(10, 6)) || v.Balance != -30000 {
		t.Fatalf("violation = %+v, want 10.06 −30000", v)
	}
}

func TestCheckRunningMoveWithdrawalEarlier(t *testing.T) {
	before := []Event{ev(day(1, 6), 100000), ev(day(10, 6), -80000)}
	after := []Event{ev(day(1, 6), 100000), ev(day(31, 5), -80000)}
	v := CheckRunning(before, after)
	if v == nil || !v.At.Equal(day(31, 5)) || v.Balance != -80000 {
		t.Fatalf("violation = %+v, want 31.05 −80000 (final balance positive)", v)
	}
}

func TestCheckRunningExistingNegativeHistory(t *testing.T) {
	// repayment recorded before the purchase it repays (migrated data)
	hist := []Event{ev(day(5, 6), -314600), ev(day(15, 6), 500000)}
	if v := CheckRunning(hist, hist); v != nil {
		t.Errorf("unchanged history flagged: %+v", v)
	}
	// unrelated improvement: a purchase earlier lifts the dip
	better := append([]Event{ev(day(1, 6), 10000)}, hist...)
	if v := CheckRunning(hist, better); v != nil {
		t.Errorf("improvement flagged: %+v", v)
	}
	// deeper dip
	worse := []Event{ev(day(5, 6), -400000), ev(day(15, 6), 500000)}
	if v := CheckRunning(hist, worse); v == nil || v.Balance != -400000 {
		t.Errorf("deeper dip not flagged: %+v", v)
	}
}

func TestCheckRunningSameDayOrder(t *testing.T) {
	at := day(10, 6)
	deposit := Event{At: at, Seq: at.Add(time.Minute), Delta: 50000}
	withdraw := Event{At: at, Seq: at, Delta: -50000} // entered first
	v := CheckRunning(nil, []Event{deposit, withdraw})
	if v == nil || v.Balance != -50000 {
		t.Errorf("same-day order: %+v", v)
	}
	withdraw.Seq = at.Add(2 * time.Minute)
	if v := CheckRunning(nil, []Event{deposit, withdraw}); v != nil {
		t.Errorf("deposit first should pass: %+v", v)
	}
}

func TestBalanceDeltas(t *testing.T) {
	if d, ok := SavingsDelta(KindSavingsWithdrawal, 5); !ok || d != -5 {
		t.Error("withdrawal")
	}
	if _, ok := SavingsDelta(KindExpense, 5); ok {
		t.Error("expense affects savings")
	}
	if d, ok := DebtDelta(KindCreditRepayment, 5); !ok || d != -5 {
		t.Error("repayment")
	}
	if d, ok := DebtDelta(KindCreditPurchase, 5); !ok || d != 5 {
		t.Error("purchase")
	}
}
