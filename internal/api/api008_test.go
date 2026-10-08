package api

import (
	"math"
	"testing"
	"time"

	"expense-bot/internal/ledger"
)

// Tests for feature 008: Summary.pace and Summary.quarter for the redesigned home screen.

func TestSummaryPaceQuarter(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.do(t, "POST", "/api/session", auth, "")
	at := func(m, d int) time.Time { return time.Date(2026, time.Month(m), d, 9, 0, 0, 0, ledger.Location) }
	e.mem.seed(uid, "s1", ledger.KindIncome, "", 600000, "", at(9, 20)) // carry-over source
	e.mem.seed(uid, "e1", ledger.KindExpense, "c_food", 100000, "", at(10, 3))
	e.mem.seed(uid, "e2", ledger.KindExpense, "c_food", 200000, "", at(10, 9))

	// single payment, 10 October
	freeze(t, 2026, 10, 10, 12)
	s := summaryOf(t, e, auth, "")
	p, _ := s["pace"].(map[string]any)
	if p == nil || p["period"] != "month" || num(p["spent"]) != num(s["expense"]) || num(p["budget"]) != num(s["carryOver"])+num(s["income"]) {
		t.Fatalf("single pace = %v (summary expense %v carry %v income %v)", p, s["expense"], s["carryOver"], s["income"])
	}
	if el := p["elapsed"].(float64); math.Abs(el-10.0/31) > 1e-9 {
		t.Errorf("elapsed = %v", el)
	}
	q, _ := s["quarter"].(map[string]any)
	if q == nil || q["name"] != "Q4 2026" || num(q["daysLeft"]) != 83 {
		t.Errorf("quarter = %v", q)
	}

	// two payments, 10th: period 1–14, start balance 600 000, spent 300 000
	e.do(t, "PATCH", "/api/profile", auth, `{"salaryMode":"split"}`)
	s = summaryOf(t, e, auth, "")
	p, _ = s["pace"].(map[string]any)
	if p == nil || p["period"] != "advance" || num(p["spent"]) != 300000 || num(p["budget"]) != 600000 {
		t.Fatalf("split pace = %v", p)
	}
	if el := p["elapsed"].(float64); math.Round(el*100) != 71 {
		t.Errorf("split elapsed = %v", el)
	}

	// past month: no pace, no quarter
	s = summaryOf(t, e, auth, "2026-09")
	if s["pace"] != nil || s["quarter"] != nil {
		t.Errorf("past month pace/quarter = %v / %v", s["pace"], s["quarter"])
	}
}

func TestSummaryPaceNoBudget(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.do(t, "POST", "/api/session", auth, "")
	freeze(t, 2026, 10, 10, 12)
	if s := summaryOf(t, e, auth, ""); s["pace"] != nil {
		t.Errorf("no budget → pace %v, want null", s["pace"])
	}
}
