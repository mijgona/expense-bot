package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"expense-bot/internal/config"
	"expense-bot/internal/ledger"
)

const botToken = "1:TEST"

func initData(userID int64) string {
	vals := url.Values{}
	vals.Set("auth_date", strconv.FormatInt(time.Now().Unix(), 10))
	vals.Set("user", `{"id":`+strconv.FormatInt(userID, 10)+`,"first_name":"Алиса","username":"alice"}`)
	pairs := []string{}
	for k, v := range vals {
		pairs = append(pairs, k+"="+v[0])
	}
	sort.Strings(pairs)
	sec := hmac.New(sha256.New, []byte("WebAppData"))
	sec.Write([]byte(botToken))
	h := hmac.New(sha256.New, sec.Sum(nil))
	h.Write([]byte(strings.Join(pairs, "\n")))
	vals.Set("hash", hex.EncodeToString(h.Sum(nil)))
	return vals.Encode()
}

type testEnv struct {
	h   http.Handler
	cfg *config.Config
	mem *memStore
}

func newEnv(allowed ...int64) testEnv {
	cfg := &config.Config{BotToken: botToken, Salary: 16000, AllowedUserIDs: allowed, WebAppURL: "https://app.example"}
	mem := newMem()
	return testEnv{h: New(mem, cfg, nil).Handler(), cfg: cfg, mem: mem}
}

func (e testEnv) do(t *testing.T, method, path, auth, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", "tma "+auth)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func errCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func TestAuth(t *testing.T) {
	e := newEnv()
	if code, body := e.do(t, "GET", "/api/summary", "", ""); code != 401 || errCode(body) != "unauthorized" {
		t.Errorf("no auth: %d %v", code, body)
	}
	if code, _ := e.do(t, "GET", "/api/summary", "user=x&hash=bad", ""); code != 401 {
		t.Errorf("bad hash: %d", code)
	}
	if code, _ := e.do(t, "GET", "/api/summary", initData(42), ""); code != 200 {
		t.Errorf("valid: %d", code)
	}
	if code, _ := e.do(t, "GET", "/health", "", ""); code != 200 {
		t.Errorf("health: %d", code)
	}
	restricted := newEnv(7)
	if code, body := restricted.do(t, "GET", "/api/summary", initData(42), ""); code != 403 || errCode(body) != "forbidden" {
		t.Errorf("not allowed: %d %v", code, body)
	}
}

func TestCORS(t *testing.T) {
	e := newEnv()
	req := httptest.NewRequest("OPTIONS", "/api/session", nil)
	req.Header.Set("Origin", "https://app.example")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Errorf("preflight: %d %v", rec.Code, rec.Header())
	}
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("foreign origin allowed")
	}
}

func TestSessionRegistersOnce(t *testing.T) {
	e := newEnv()
	auth := initData(42)
	_, b := e.do(t, "POST", "/api/session", auth, "")
	if u := b["user"].(map[string]any); u["isNew"] != true || u["firstName"] != "Алиса" {
		t.Errorf("first session: %v", b)
	}
	if b["salary"].(float64) != 1600000 || len(b["categories"].([]any)) != 14 {
		t.Errorf("bootstrap: %v", b)
	}
	_, b = e.do(t, "POST", "/api/session", auth, "")
	if b["user"].(map[string]any)["isNew"] != false {
		t.Error("second session registered again")
	}
}

const uuid1 = "11111111-1111-4111-8111-111111111111"
const uuid2 = "22222222-2222-4222-8222-222222222222"

func TestTransactionsFlow(t *testing.T) {
	e := newEnv()
	auth := initData(42)

	code, b := e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuid1+`","kind":"expense","amount":35000,"category":"c_transport","note":"такси"}`)
	if code != 201 {
		t.Fatalf("create: %d %v", code, b)
	}
	if rem := b["summary"].(map[string]any)["remaining"].(float64); rem != -35000 {
		t.Errorf("remaining = %v", rem)
	}
	// idempotent retry
	if code, _ := e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuid1+`","kind":"expense","amount":35000,"category":"c_transport"}`); code != 200 {
		t.Errorf("retry: %d, want 200", code)
	}
	_, s := e.do(t, "GET", "/api/summary", auth, "")
	if s["expense"].(float64) != 35000 {
		t.Errorf("duplicate counted: %v", s["expense"])
	}

	cases := []struct{ body, field string }{
		{`{"clientId":"x","kind":"expense","amount":1,"category":"c_food"}`, "clientId"},
		{`{"clientId":"` + uuid2 + `","kind":"refund","amount":1}`, "kind"},
		{`{"clientId":"` + uuid2 + `","kind":"expense","amount":0,"category":"c_food"}`, "amount"},
		{`{"clientId":"` + uuid2 + `","kind":"expense","amount":5,"category":"Нет такой"}`, "category"},
		{`{"clientId":"` + uuid2 + `","kind":"income","amount":5,"category":"c_food"}`, "category"},
	}
	for _, c := range cases {
		code, b := e.do(t, "POST", "/api/transactions", auth, c.body)
		errObj, _ := b["error"].(map[string]any)
		if code != 400 || errObj["field"] != c.field {
			t.Errorf("%s: %d %v, want 400 field=%s", c.body, code, b, c.field)
		}
	}

	code, b = e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuid2+`","kind":"savings_withdrawal","amount":100}`)
	if code != 422 || errCode(b) != "insufficient_savings" {
		t.Errorf("withdraw: %d %v", code, b)
	}
	code, b = e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuid2+`","kind":"credit_repayment","amount":100}`)
	if code != 422 || errCode(b) != "exceeds_debt" {
		t.Errorf("repay: %d %v", code, b)
	}

	if code, _ := e.do(t, "GET", "/api/summary?month=2999-01", auth, ""); code != 400 {
		t.Errorf("future month: %d", code)
	}
	if code, b := e.do(t, "GET", "/api/transactions?month="+ledger.MonthKey(ledger.Now()), auth, ""); code != 200 || len(b["items"].([]any)) != 1 {
		t.Errorf("list: %d %v", code, b)
	}
}

func TestGoals(t *testing.T) {
	e := newEnv()
	auth := initData(42)
	q := ledger.AllowedQuarters(ledger.Now())[0]
	code, b := e.do(t, "POST", "/api/goals", auth, `{"clientId":"`+uuid1+`","name":"Отпуск","target":300000,"quarter":"`+q+`"}`)
	if code != 201 || b["status"] != "active" {
		t.Errorf("create goal: %d %v", code, b)
	}
	if code, _ := e.do(t, "POST", "/api/goals", auth, `{"clientId":"`+uuid2+`","name":"X","target":1,"quarter":"Q1 2000"}`); code != 400 {
		t.Errorf("past quarter: %d", code)
	}
	goal := func() map[string]any {
		_, b := e.do(t, "GET", "/api/goals", auth, "")
		return b["items"].([]any)[0].(map[string]any)
	}
	// A deposit without a goal doesn't fill any goal.
	e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuid2+`","kind":"savings_deposit","amount":50000}`)
	if g := goal(); num(g["saved"]) != 0 || g["progress"].(float64) != 0 {
		t.Errorf("unlinked deposit filled the goal: %v", g)
	}
	// A deposit to the goal does.
	const dep = "33333333-3333-4333-8333-333333333333"
	code, b = e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+dep+`","kind":"savings_deposit","amount":150000,"goalId":"`+uuid1+`"}`)
	if code != 201 || b["transaction"].(map[string]any)["goalId"] != uuid1 {
		t.Fatalf("deposit to goal: %d %v", code, b)
	}
	if g := goal(); num(g["saved"]) != 150000 || g["progress"].(float64) != 0.5 {
		t.Errorf("goal after deposit = %v, want saved 150000, progress 0.5", g)
	}
	// Only deposits take a goal, and only an existing active one.
	if code, b := e.do(t, "POST", "/api/transactions", auth, `{"clientId":"44444444-4444-4444-8444-444444444444","kind":"savings_withdrawal","amount":100,"goalId":"`+uuid1+`"}`); code != 400 || b["error"].(map[string]any)["field"] != "goalId" {
		t.Errorf("withdrawal with goal: %d %v", code, b)
	}
	if code, _ := e.do(t, "POST", "/api/transactions", auth, `{"clientId":"55555555-5555-4555-8555-555555555555","kind":"savings_deposit","amount":100,"goalId":"nope"}`); code != 400 {
		t.Errorf("unknown goal: %d", code)
	}
	// Unlinking the deposit empties the goal again.
	if code, b := e.do(t, "PATCH", "/api/transactions/"+dep, auth, `{"version":1,"requestId":"66666666-6666-4666-8666-666666666666","goalId":""}`); code != 200 || b["transaction"].(map[string]any)["goalId"] != nil {
		t.Errorf("unlink: %d %v", code, b)
	}
	if g := goal(); num(g["saved"]) != 0 {
		t.Errorf("goal after unlink = %v", g)
	}
}

func TestAdvisorUnavailable(t *testing.T) {
	e := newEnv()
	if code, b := e.do(t, "POST", "/api/advisor/report", initData(42), ""); code != 503 || errCode(b) != "advisor_unavailable" {
		t.Errorf("advisor: %d %v", code, b)
	}
}
