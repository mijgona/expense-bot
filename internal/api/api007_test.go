package api

import (
	"testing"
	"time"

	"expense-bot/internal/ledger"
)

// Tests for feature 007: salary in two payments.

func freeze(t *testing.T, y, m, d, hh int) {
	t.Helper()
	at := time.Date(y, time.Month(m), d, hh, 0, 0, 0, ledger.Location)
	t.Cleanup(ledger.SetNowForTest(func() time.Time { return at }))
}

func payout(items []any, month, kind string) map[string]any {
	for _, it := range items {
		if m := it.(map[string]any); m["month"] == month && m["kind"] == kind {
			return m
		}
	}
	return nil
}

func TestProfileSchedule(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.do(t, "POST", "/api/session", auth, "")
	_, p := e.do(t, "GET", "/api/profile", auth, "")
	adv := p["advance"].(map[string]any)
	if p["salaryMode"] != "single" || num(adv["value"]) != 800000 || adv["isDefault"] != true || num(p["rest"]) != 1600000 || p["salaryReminders"] != true {
		t.Fatalf("defaults: %v", p)
	}
	code, p := e.do(t, "PATCH", "/api/profile", auth, `{"salaryMode":"split","advance":600000}`)
	if code != 200 || num(p["rest"]) != 1000000 || num(p["advance"].(map[string]any)["value"]) != 600000 {
		t.Fatalf("split: %d %v", code, p)
	}
	for _, body := range []string{`{"advance":0}`, `{"advance":1600000}`, `{"advance":2000000}`} {
		code, b := e.do(t, "PATCH", "/api/profile", auth, body)
		if code != 400 || b["error"].(map[string]any)["field"] != "advance" {
			t.Errorf("%s: %d %v", body, code, b)
		}
	}
	e.do(t, "PATCH", "/api/profile", auth, `{"advance":1200000,"salary":1600000}`)
	_, p = e.do(t, "PATCH", "/api/profile", auth, `{"salary":1000000}`)
	if a := p["advance"].(map[string]any); num(a["value"]) != 500000 || a["isDefault"] != true {
		t.Errorf("advance after salary drop: %v", a)
	}
	_, p = e.do(t, "PATCH", "/api/profile", auth, `{"advance":null,"salary":1600000}`)
	if num(p["advance"].(map[string]any)["value"]) != 800000 {
		t.Errorf("advance reset: %v", p["advance"])
	}
	_, p = e.do(t, "PATCH", "/api/profile", auth, `{"salaryReminders":false}`)
	if p["salaryReminders"] != false {
		t.Error("reminders off")
	}
	if code, _ := e.do(t, "PATCH", "/api/profile", auth, `{"salaryMode":"weekly"}`); code != 400 {
		t.Errorf("bad mode: %d", code)
	}
}

func TestPayouts(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.do(t, "POST", "/api/session", auth, "")
	e.do(t, "PATCH", "/api/profile", auth, `{"salaryMode":"split"}`)

	freeze(t, 2026, 10, 10, 12)
	_, b := e.do(t, "GET", "/api/payouts", auth, "")
	if p := payout(b["items"].([]any), "2026-10", "advance"); p["status"] != "upcoming" || p["amount"] != nil || p["payday"] != "2026-10-15" {
		t.Errorf("10th advance: %v", p)
	}

	freeze(t, 2026, 10, 15, 12)
	_, b = e.do(t, "GET", "/api/payouts", auth, "")
	if p := payout(b["items"].([]any), "2026-10", "advance"); p["status"] != "due" {
		t.Errorf("15th advance: %v", p)
	}
	code, r := e.do(t, "POST", "/api/payouts/2026-10/advance/record", auth, `{"amount":750000}`)
	tx, _ := r["transaction"].(map[string]any)
	if code != 201 || tx["id"] != "p_2026-10_advance" || tx["note"] != "Аванс" || tx["kind"] != "income" || tx["month"] != "2026-10" {
		t.Fatalf("record: %d %v", code, r)
	}
	if code, _ := e.do(t, "POST", "/api/payouts/2026-10/advance/record", auth, `{"amount":750000}`); code != 200 {
		t.Errorf("replay: %d", code)
	}
	if s := summaryOf(t, e, auth, "2026-10"); num(s["income"]) != 750000 {
		t.Errorf("income counted twice: %v", s["income"])
	}
	_, b = e.do(t, "GET", "/api/payouts", auth, "")
	if p := payout(b["items"].([]any), "2026-10", "advance"); p["status"] != "recorded" || p["transactionId"] != "p_2026-10_advance" {
		t.Errorf("recorded: %v", p)
	}

	e.do(t, "DELETE", "/api/transactions/p_2026-10_advance?version=1", auth, "")
	_, b = e.do(t, "GET", "/api/payouts", auth, "")
	if p := payout(b["items"].([]any), "2026-10", "advance"); p["status"] != "due" {
		t.Errorf("after delete: %v", p)
	}
	if code, d := e.do(t, "POST", "/api/payouts/2026-10/advance/dismiss", auth, ""); code != 200 || d["status"] != "dismissed" {
		t.Errorf("dismiss: %d %v", code, d)
	}

	if code, _ := e.do(t, "POST", "/api/payouts/2026-10/full/record", auth, `{"amount":1}`); code != 404 {
		t.Errorf("full in split: %d", code)
	}
	if code, _ := e.do(t, "POST", "/api/payouts/2026-10/rest/record", auth, `{"amount":1,"date":"2026-10-20"}`); code != 400 {
		t.Errorf("future date: %d", code)
	}
	if code, _ := e.do(t, "POST", "/api/payouts/2026-13/rest/record", auth, `{"amount":1}`); code != 400 {
		t.Errorf("bad month: %d", code)
	}
	if code, _ := e.do(t, "POST", "/api/payouts/2026-10/rest/record", auth, `{"amount":1}`); code != 400 {
		t.Errorf("rest before its payday without date: %d", code)
	}
}

func TestPayoutsPreviousMonthGrace(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.do(t, "POST", "/api/session", auth, "")

	freeze(t, 2026, 11, 2, 9)
	_, b := e.do(t, "GET", "/api/payouts", auth, "")
	if p := payout(b["items"].([]any), "2026-10", "full"); p == nil || p["status"] != "due" {
		t.Errorf("Nov 2: previous full = %v", p)
	}
	code, r := e.do(t, "POST", "/api/payouts/2026-10/full/record", auth, `{"amount":1600000}`)
	if code != 201 || r["transaction"].(map[string]any)["month"] != "2026-10" {
		t.Errorf("record previous month: %d %v", code, r)
	}

	freeze(t, 2026, 11, 6, 9)
	_, b = e.do(t, "GET", "/api/payouts", auth, "")
	if p := payout(b["items"].([]any), "2026-10", "full"); p != nil {
		t.Errorf("Nov 6 still offers previous month: %v", p)
	}
}

func TestSummaryNextPayday(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.do(t, "POST", "/api/session", auth, "")
	e.do(t, "PATCH", "/api/profile", auth, `{"salaryMode":"split"}`)
	e.mem.seed(uid, "inc", ledger.KindIncome, "", 400000, "", time.Date(2026, 10, 1, 9, 0, 0, 0, ledger.Location))

	freeze(t, 2026, 10, 10, 12)
	s := summaryOf(t, e, auth, "")
	np, _ := s["nextPayday"].(map[string]any)
	if np == nil || np["date"] != "2026-10-15" || np["kind"] != "advance" || num(np["daysLeft"]) != 5 || num(s["dailyBudget"]) != 80000 {
		t.Errorf("10th: nextPayday %v dailyBudget %v", np, s["dailyBudget"])
	}
	freeze(t, 2026, 10, 15, 12)
	s = summaryOf(t, e, auth, "")
	if np, _ := s["nextPayday"].(map[string]any); np == nil || np["date"] != "2026-10-31" || num(np["daysLeft"]) != 16 {
		t.Errorf("15th: %v", s["nextPayday"])
	}
	if s := summaryOf(t, e, auth, "2026-09"); s["nextPayday"] != nil {
		t.Errorf("past month nextPayday = %v", s["nextPayday"])
	}
	e.do(t, "PATCH", "/api/profile", auth, `{"salaryMode":"single"}`)
	s = summaryOf(t, e, auth, "")
	if s["nextPayday"] != nil || num(s["daysLeft"]) != 17 {
		t.Errorf("single: nextPayday %v daysLeft %v", s["nextPayday"], s["daysLeft"])
	}
}
