package ledger

import (
	"math"
	"testing"
	"time"
)

func dt(m, d, hh int) time.Time { return time.Date(2026, time.Month(m), d, hh, 0, 0, 0, Location) }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPaceSingle(t *testing.T) {
	p := Pace(false, dt(10, 8, 12), 1778590, 100, 1013000, nil)
	if p == nil || p.Spent != 1013000 || p.Budget != 1778690 || !near(p.Elapsed, 8.0/31) || p.Period != "month" {
		t.Fatalf("single = %+v", p)
	}
	if pct := math.Round(float64(p.Spent) / float64(p.Budget) * 100); pct != 57 {
		t.Errorf("spent %% = %v, want 57", pct)
	}
	if pct := math.Round(p.Elapsed * 100); pct != 26 {
		t.Errorf("elapsed %% = %v, want 26", pct)
	}
	if p := Pace(false, dt(10, 8, 12), 0, 0, 5000, nil); p != nil {
		t.Errorf("budget ≤ 0 → %+v, want nil", p)
	}
}

func TestPaceSplitAdvancePeriod(t *testing.T) {
	txs := []PaceTx{
		{At: dt(10, 3, 9), Kind: KindExpense, Amount: 100000},
		{At: dt(10, 9, 9), Kind: KindExpense, Amount: 200000},
		{At: dt(10, 9, 10), Kind: KindSavingsDeposit, Amount: 50000}, // not spending
	}
	p := Pace(true, dt(10, 10, 12), 600000, 0, 300000, txs)
	if p == nil || p.Spent != 300000 || p.Budget != 600000 || !near(p.Elapsed, 10.0/14) || p.Period != "advance" {
		t.Fatalf("advance period = %+v", p)
	}
	if math.Round(p.Elapsed*100) != 71 || math.Round(float64(p.Spent)/float64(p.Budget)*100) != 50 {
		t.Errorf("percentages = %v / %v", p.Elapsed, float64(p.Spent)/float64(p.Budget))
	}
}

func TestPaceSplitRestPeriod(t *testing.T) {
	txs := []PaceTx{
		{At: dt(10, 3, 9), Kind: KindIncome, Amount: 1000000},
		{At: dt(10, 5, 9), Kind: KindExpense, Amount: 200000},
		{At: dt(10, 16, 9), Kind: KindIncome, Amount: 500000},
		{At: dt(10, 18, 9), Kind: KindExpense, Amount: 100000},
	}
	p := Pace(true, dt(10, 20, 12), 50000, 1500000, 300000, txs)
	// start balance = 50 000 + (1 000 000 − 200 000); + income in period 500 000
	if p == nil || p.Budget != 1350000 || p.Spent != 100000 || !near(p.Elapsed, 6.0/17) || p.Period != "rest" {
		t.Fatalf("rest period = %+v", p)
	}
}

func TestQuarter(t *testing.T) {
	name, start, end := QuarterBounds(dt(10, 8, 12))
	if name != "Q4 2026" || !start.Equal(dt(10, 1, 0)) || !end.Equal(dt(12, 31, 0)) {
		t.Errorf("bounds = %s %v %v", name, start, end)
	}
	n, elapsed, left := QuarterProgress(dt(10, 8, 12))
	if n != "Q4 2026" || left != 85 || !near(elapsed, 8.0/92) {
		t.Errorf("progress = %s %v %d", n, elapsed, left)
	}
	if _, _, left := QuarterProgress(dt(12, 31, 23)); left != 1 {
		t.Errorf("last day left = %d", left)
	}
	if n, _, _ := QuarterProgress(dt(1, 1, 0)); n != "Q1 2026" {
		t.Errorf("Jan 1 = %s", n)
	}
}
