package api

import (
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"expense-bot/internal/ledger"
)

// Tests for feature 005: history, edit, delete, profile, goals.

const uid = 42

func monthsAgo(n int) time.Time {
	now := ledger.Now()
	return time.Date(now.Year(), now.Month()-time.Month(n), 10, 12, 0, 0, 0, ledger.Location)
}

func num(v any) int64 { f, _ := v.(float64); return int64(f) }

func summaryOf(t *testing.T, e testEnv, auth, month string) map[string]any {
	t.Helper()
	path := "/api/summary"
	if month != "" {
		path += "?month=" + month
	}
	code, b := e.do(t, "GET", path, auth, "")
	if code != 200 {
		t.Fatalf("summary %s: %d %v", month, code, b)
	}
	return b
}

func catSpent(s map[string]any, name string) int64 {
	for _, c := range s["categories"].([]any) {
		m := c.(map[string]any)
		if m["id"] == name {
			return num(m["spent"])
		}
	}
	return -1
}

func uuidN(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func TestHistory(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	for i := 0; i < 120; i++ {
		at := monthsAgo(i % 3).Add(time.Duration(i) * time.Minute)
		switch i % 4 {
		case 0:
			e.mem.seed(uid, "t"+strconv.Itoa(i), ledger.KindExpense, "c_food", 1000, "обед", at)
		case 1:
			e.mem.seed(uid, "t"+strconv.Itoa(i), ledger.KindExpense, "c_transport", 500, "Такси домой", at)
		case 2:
			e.mem.seed(uid, "t"+strconv.Itoa(i), ledger.KindIncome, "", 5000, "", at)
		case 3:
			e.mem.seed(uid, "t"+strconv.Itoa(i), ledger.KindSavingsDeposit, "", 300, "", at)
		}
	}

	// paging through everything
	seen, cursor, pages := 0, "", 0
	var prev time.Time
	for {
		path := "/api/transactions?limit=50"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		code, b := e.do(t, "GET", path, auth, "")
		if code != 200 {
			t.Fatalf("page: %d %v", code, b)
		}
		for _, it := range b["items"].([]any) {
			at, _ := time.Parse(time.RFC3339Nano, it.(map[string]any)["occurredAt"].(string))
			if !prev.IsZero() && at.After(prev) {
				t.Fatal("not newest first")
			}
			prev = at
			seen++
		}
		pages++
		nc, _ := b["nextCursor"].(string)
		if nc == "" {
			break
		}
		cursor = nc
	}
	if seen != 120 || pages != 3 {
		t.Errorf("seen %d in %d pages, want 120 in 3", seen, pages)
	}

	// group + category + month, exact total
	m := ledger.MonthKey(monthsAgo(0))
	_, b := e.do(t, "GET", "/api/transactions?group=expense&category=c_food&month="+m, auth, "")
	items := b["items"].([]any)
	var sum int64
	for _, it := range items {
		x := it.(map[string]any)
		if x["kind"] != "expense" || x["category"] != "c_food" || x["month"] != m {
			t.Fatalf("filter leak: %v", x)
		}
		sum += num(x["amount"])
	}
	if len(items) == 0 || num(b["total"]) != sum || b["totalExact"] != true {
		t.Errorf("filtered total = %v (rows %d), exact %v", b["total"], sum, b["totalExact"])
	}
	_, b = e.do(t, "GET", "/api/transactions?group=savings", auth, "")
	for _, it := range b["items"].([]any) {
		if k := it.(map[string]any)["kind"]; k != "savings_deposit" && k != "savings_withdrawal" {
			t.Fatalf("savings group leak: %v", k)
		}
	}

	// search: case-insensitive, total not exact
	_, b = e.do(t, "GET", "/api/transactions?q=такси", auth, "")
	if len(b["items"].([]any)) == 0 || b["totalExact"] != false {
		t.Errorf("search: %d items, exact %v", len(b["items"].([]any)), b["totalExact"])
	}

	// empty result
	_, b = e.do(t, "GET", "/api/transactions?q=несуществует", auth, "")
	if len(b["items"].([]any)) != 0 {
		t.Error("expected empty")
	}

	// bad inputs
	if code, _ := e.do(t, "GET", "/api/transactions?group=nope", auth, ""); code != 400 {
		t.Errorf("bad group: %d", code)
	}

	// months
	_, b = e.do(t, "GET", "/api/months", auth, "")
	ms := b["items"].([]any)
	if len(ms) != 3 || ms[0].(map[string]any)["month"] != m {
		t.Errorf("months = %v", ms)
	}
}

func TestEditTransaction(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	prevMonth := ledger.MonthKey(monthsAgo(1))
	curMonth := ledger.MonthKey(ledger.Now())
	e.mem.seed(uid, "x1", ledger.KindExpense, "c_transport", 35000, "такси", monthsAgo(1))
	e.mem.seed(uid, "inc", ledger.KindIncome, "", 1000000, "", monthsAgo(1))
	before := summaryOf(t, e, auth, curMonth)

	// same-month edit: 350 Транспорт → 530 Еда
	code, b := e.do(t, "PATCH", "/api/transactions/x1", auth,
		`{"version":1,"requestId":"`+uuidN(1)+`","amount":53000,"category":"c_food"}`)
	if code != 200 || num(b["transaction"].(map[string]any)["version"]) != 2 || b["transaction"].(map[string]any)["editedAt"] == nil {
		t.Fatalf("patch: %d %v", code, b)
	}
	s := summaryOf(t, e, auth, prevMonth)
	if catSpent(s, "c_transport") != 0 || catSpent(s, "c_food") != 53000 || num(s["expense"]) != 53000 {
		t.Errorf("prev month after edit: Транспорт %d, Еда %d, expense %v", catSpent(s, "c_transport"), catSpent(s, "c_food"), s["expense"])
	}
	after := summaryOf(t, e, auth, curMonth)
	if num(after["carryOver"]) != num(before["carryOver"])-18000 {
		t.Errorf("carry-over %v → %v, want −18000", before["carryOver"], after["carryOver"])
	}

	// idempotent retry with the same requestId
	code, b = e.do(t, "PATCH", "/api/transactions/x1", auth,
		`{"version":1,"requestId":"`+uuidN(1)+`","amount":53000,"category":"c_food"}`)
	if code != 200 || num(b["transaction"].(map[string]any)["version"]) != 2 {
		t.Errorf("retry: %d version %v", code, b["transaction"])
	}

	// stale version from another device
	code, b = e.do(t, "PATCH", "/api/transactions/x1", auth, `{"version":1,"requestId":"`+uuidN(2)+`","note":"x"}`)
	if code != 409 || errCode(b) != "conflict" || num(b["error"].(map[string]any)["current"].(map[string]any)["version"]) != 2 {
		t.Errorf("conflict: %d %v", code, b)
	}

	// move the date into the current month
	today := ledger.Now().Format("2006-01-02")
	code, b = e.do(t, "PATCH", "/api/transactions/x1", auth, `{"version":2,"requestId":"`+uuidN(3)+`","date":"`+today+`"}`)
	if code != 200 || b["transaction"].(map[string]any)["month"] != curMonth {
		t.Fatalf("move date: %d %v", code, b)
	}
	if s := summaryOf(t, e, auth, prevMonth); num(s["expense"]) != 0 {
		t.Errorf("prev month still has expense %v", s["expense"])
	}
	if s := summaryOf(t, e, auth, curMonth); catSpent(s, "c_food") != 53000 {
		t.Errorf("current month Еда = %d", catSpent(s, "c_food"))
	}

	// validation
	tomorrow := ledger.Now().AddDate(0, 0, 1).Format("2006-01-02")
	cases := []struct{ body, field string }{
		{`{"version":3,"requestId":"` + uuidN(4) + `","date":"` + tomorrow + `"}`, "date"},
		{`{"version":3,"requestId":"` + uuidN(4) + `","category":"Нет такой"}`, "category"},
		{`{"version":3,"requestId":"` + uuidN(4) + `","amount":0}`, "amount"},
		{`{"version":3,"requestId":"bad"}`, "requestId"},
	}
	for _, c := range cases {
		code, b := e.do(t, "PATCH", "/api/transactions/x1", auth, c.body)
		if code != 400 || b["error"].(map[string]any)["field"] != c.field {
			t.Errorf("%s: %d %v", c.body, code, b)
		}
	}
	if code, _ := e.do(t, "PATCH", "/api/transactions/inc", auth, `{"version":1,"requestId":"`+uuidN(5)+`","category":"c_food"}`); code != 400 {
		t.Errorf("category on income: %d", code)
	}
	if code, _ := e.do(t, "PATCH", "/api/transactions/x1", auth, `{"version":3,"requestId":"`+uuidN(6)+`","kind":"income"}`); code != 400 {
		t.Errorf("kind change accepted: %d", code)
	}
	if code, _ := e.do(t, "PATCH", "/api/transactions/x1", initData(7), `{"version":3,"requestId":"`+uuidN(7)+`","note":"x"}`); code != 404 {
		t.Errorf("other user's record: %d", code)
	}
}

func TestEditBalanceRule(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	d1 := monthsAgo(1)
	e.mem.seed(uid, "dep", ledger.KindSavingsDeposit, "", 100000, "", d1)
	e.mem.seed(uid, "wd", ledger.KindSavingsWithdrawal, "", 80000, "", d1.Add(48*time.Hour))

	code, b := e.do(t, "PATCH", "/api/transactions/dep", auth, `{"version":1,"requestId":"`+uuidN(1)+`","amount":50000}`)
	errObj, _ := b["error"].(map[string]any)
	if code != 422 || errCode(b) != "insufficient_savings" || errObj["at"] == nil || num(errObj["balance"]) != -30000 {
		t.Fatalf("lower deposit: %d %v", code, b)
	}
	// moving the withdrawal before the deposit is rejected although the final balance is positive
	early := d1.AddDate(0, 0, -2).Format("2006-01-02")
	if code, b := e.do(t, "PATCH", "/api/transactions/wd", auth, `{"version":1,"requestId":"`+uuidN(2)+`","date":"`+early+`"}`); code != 422 {
		t.Errorf("withdrawal before deposit: %d %v", code, b)
	}
	// already-negative credit history doesn't block unrelated edits
	e.mem.seed(uid, "rep", ledger.KindCreditRepayment, "", 314600, "", d1)
	e.mem.seed(uid, "buy", ledger.KindCreditPurchase, "c_food", 500000, "", d1.Add(240*time.Hour))
	e.mem.seed(uid, "food", ledger.KindExpense, "c_food", 1000, "", d1)
	if code, b := e.do(t, "PATCH", "/api/transactions/food", auth, `{"version":1,"requestId":"`+uuidN(3)+`","amount":2000}`); code != 200 {
		t.Errorf("unrelated edit blocked: %d %v", code, b)
	}
	if code, _ := e.do(t, "PATCH", "/api/transactions/rep", auth, `{"version":1,"requestId":"`+uuidN(4)+`","amount":400000}`); code != 422 {
		t.Errorf("deeper dip accepted: %d", code)
	}
	// back-dated create follows the same rule
	if code, b := e.do(t, "POST", "/api/transactions", auth,
		`{"clientId":"`+uuidN(5)+`","kind":"savings_withdrawal","amount":10000,"date":"`+early+`"}`); code != 422 {
		t.Errorf("back-dated withdrawal: %d %v", code, b)
	}
}

func TestDeleteTransaction(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	cur := ledger.MonthKey(ledger.Now())
	base := summaryOf(t, e, auth, cur)
	e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuidN(1)+`","kind":"expense","amount":10000,"category":"c_food"}`)

	if code, _ := e.do(t, "DELETE", "/api/transactions/"+uuidN(1)+"?version=2", auth, ""); code != 409 {
		t.Errorf("stale delete: %d", code)
	}
	code, b := e.do(t, "DELETE", "/api/transactions/"+uuidN(1)+"?version=1", auth, "")
	if code != 200 || num(b["summary"].(map[string]any)["remaining"]) != num(base["remaining"]) {
		t.Fatalf("delete: %d %v", code, b)
	}
	if code, _ := e.do(t, "DELETE", "/api/transactions/"+uuidN(1)+"?version=1", auth, ""); code != 204 {
		t.Errorf("repeat delete: %d", code)
	}

	// deleting a deposit that a later withdrawal depends on
	e.mem.seed(uid, "dep", ledger.KindSavingsDeposit, "", 100000, "", monthsAgo(1))
	e.mem.seed(uid, "wd", ledger.KindSavingsWithdrawal, "", 80000, "", monthsAgo(1).Add(time.Hour))
	if code, b := e.do(t, "DELETE", "/api/transactions/dep?version=1", auth, ""); code != 422 || errCode(b) != "insufficient_savings" {
		t.Errorf("delete deposit: %d %v", code, b)
	}

	// migrated record leaves a tombstone
	e.mem.seed(uid, "m_abc", ledger.KindExpense, "c_food", 500, "", monthsAgo(1))
	e.do(t, "DELETE", "/api/transactions/m_abc?version=1", auth, "")
	if !e.mem.tombstones[uid]["m_abc"] {
		t.Error("no tombstone for migrated record")
	}
}

func TestProfile(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.do(t, "POST", "/api/session", auth, "")

	_, p := e.do(t, "GET", "/api/profile", auth, "")
	if num(p["salary"].(map[string]any)["value"]) != 1600000 || p["salary"].(map[string]any)["isDefault"] != true {
		t.Fatalf("default salary: %v", p["salary"])
	}
	if _, ok := p["limits"]; ok {
		t.Error("profile still returns limits (moved to categories in 006)")
	}
	code, p := e.do(t, "PATCH", "/api/profile", auth, `{"salary":2000000,"displayName":"Мижгона"}`)
	if code != 200 {
		t.Fatalf("patch: %d %v", code, p)
	}
	_, s := e.do(t, "POST", "/api/session", auth, "")
	if num(s["salary"]) != 2000000 || s["user"].(map[string]any)["displayName"] != "Мижгона" {
		t.Errorf("session: salary %v name %v", s["salary"], s["user"])
	}
	e.do(t, "PATCH", "/api/profile", auth, `{"salary":null}`)
	if _, s = e.do(t, "POST", "/api/session", auth, ""); num(s["salary"]) != 1600000 {
		t.Errorf("salary reset: %v", s["salary"])
	}
	for body, field := range map[string]string{
		`{"limits":{"c_food":100}}`: "limits",
		`{"salary":0}`:              "salary",
		`{"displayName":""}`:        "displayName",
	} {
		code, b := e.do(t, "PATCH", "/api/profile", auth, body)
		if code != 400 || b["error"].(map[string]any)["field"] != field {
			t.Errorf("%s: %d %v", body, code, b)
		}
	}
	_, other := e.do(t, "GET", "/api/profile", initData(7), "")
	if num(other["salary"].(map[string]any)["value"]) != 1600000 {
		t.Error("other user's salary changed")
	}
}

func TestGoalEdit(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	q := ledger.AllowedQuarters(ledger.Now())[0]
	e.do(t, "POST", "/api/goals", auth, `{"clientId":"`+uuidN(1)+`","name":"Отпуск","target":300000,"quarter":"`+q+`"}`)
	e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuidN(9)+`","kind":"savings_deposit","amount":100000}`)

	code, g := e.do(t, "PATCH", "/api/goals/"+uuidN(1), auth, `{"version":1,"requestId":"`+uuidN(2)+`","name":"Море","target":400000,"status":"done"}`)
	if code != 200 || g["name"] != "Море" || g["status"] != "done" || num(g["version"]) != 2 {
		t.Fatalf("patch goal: %d %v", code, g)
	}
	if code, g := e.do(t, "PATCH", "/api/goals/"+uuidN(1), auth, `{"version":1,"requestId":"`+uuidN(2)+`","name":"Море"}`); code != 200 || num(g["version"]) != 2 {
		t.Errorf("retry: %d %v", code, g)
	}
	if code, _ := e.do(t, "PATCH", "/api/goals/"+uuidN(1), auth, `{"version":1,"requestId":"`+uuidN(3)+`","note":"x"}`); code != 409 {
		t.Errorf("stale: %d", code)
	}
	if code, _ := e.do(t, "PATCH", "/api/goals/"+uuidN(1), auth, `{"version":2,"requestId":"`+uuidN(4)+`","quarter":"`+q+`"}`); code != 200 {
		t.Errorf("own quarter: %d", code)
	}
	if code, _ := e.do(t, "PATCH", "/api/goals/"+uuidN(1), auth, `{"version":3,"requestId":"`+uuidN(5)+`","quarter":"Q1 2000"}`); code != 400 {
		t.Errorf("past quarter: %d", code)
	}
	if code, _ := e.do(t, "DELETE", "/api/goals/"+uuidN(1)+"?version=3", auth, ""); code != 204 {
		t.Errorf("delete: %d", code)
	}
	if code, _ := e.do(t, "DELETE", "/api/goals/"+uuidN(1)+"?version=3", auth, ""); code != 204 {
		t.Errorf("repeat delete: %d", code)
	}
	if s := summaryOf(t, e, auth, ""); num(s["savingsBalance"]) != 100000 {
		t.Errorf("savings changed by goal ops: %v", s["savingsBalance"])
	}
}

func TestHistoryTotalFallback(t *testing.T) {
	e := newEnv()
	auth := initData(uid)
	e.mem.seed(uid, "a", ledger.KindExpense, "c_food", 1500, "", monthsAgo(0))
	e.mem.seed(uid, "b", ledger.KindExpense, "c_food", 2500, "", monthsAgo(0))
	e.mem.sumErr = errors.New("rpc error: code = FailedPrecondition desc = The query requires an index")
	code, b := e.do(t, "GET", "/api/transactions?group=expense&category=c_food", auth, "")
	if code != 200 || len(b["items"].([]any)) != 2 || num(b["total"]) != 4000 || b["totalExact"] != false {
		t.Errorf("fallback: %d %v", code, b)
	}
}
