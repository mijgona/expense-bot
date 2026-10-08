package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"expense-bot/internal/config"
	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

// memStore is an in-memory store.Store with the same ledger semantics as Firestore.
type memStore struct {
	mu     sync.Mutex
	users  map[int64]*store.User
	txs    map[int64]map[string]store.Transaction
	months map[int64]map[string]ledger.Month
	goals  map[int64][]store.Goal
}

func newMem() *memStore {
	return &memStore{users: map[int64]*store.User{}, txs: map[int64]map[string]store.Transaction{},
		months: map[int64]map[string]ledger.Month{}, goals: map[int64][]store.Goal{}}
}

func (m *memStore) EnsureUser(_ context.Context, id int64, fn, un string) (store.User, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[id]; ok {
		return *u, false, nil
	}
	u := &store.User{ID: id, FirstName: fn, Username: un, RegisteredAt: time.Now()}
	m.users[id] = u
	return *u, true, nil
}
func (m *memStore) GetUser(_ context.Context, id int64) (store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[id]; ok {
		return *u, nil
	}
	return store.User{ID: id}, nil
}
func (m *memStore) AddTransaction(_ context.Context, uid int64, t store.Transaction) (store.Transaction, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.txs[uid] == nil {
		m.txs[uid], m.months[uid] = map[string]store.Transaction{}, map[string]ledger.Month{}
	}
	if old, ok := m.txs[uid][t.ID]; ok {
		return old, false, nil
	}
	u := m.users[uid]
	if u == nil {
		u = &store.User{ID: uid}
		m.users[uid] = u
	}
	d := ledger.Apply(t.Kind, t.Category, t.Amount)
	if u.SavingsBalance+d.SavingsBalance < 0 {
		return store.Transaction{}, false, &store.LimitError{Err: store.ErrInsufficientSavings, Available: u.SavingsBalance}
	}
	if u.CreditDebt+d.CreditDebt < 0 {
		return store.Transaction{}, false, &store.LimitError{Err: store.ErrExceedsDebt, Available: u.CreditDebt}
	}
	m.txs[uid][t.ID] = t
	mo := m.months[uid][t.Month]
	mo.Month = t.Month
	mo.Add(d.Month)
	m.months[uid][t.Month] = mo
	u.SavingsBalance += d.SavingsBalance
	u.CreditDebt += d.CreditDebt
	return t, true, nil
}
func (m *memStore) ListTransactions(_ context.Context, uid int64, month string, limit int) ([]store.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []store.Transaction
	for _, t := range m.txs[uid] {
		if t.Month == month {
			out = append(out, t)
		}
	}
	return out, nil
}
func (m *memStore) GetMonth(_ context.Context, uid int64, month string) (ledger.Month, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mo := m.months[uid][month]
	mo.Month = month
	return mo, nil
}
func (m *memStore) MonthsBefore(_ context.Context, uid int64, month string) ([]ledger.Month, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ledger.Month
	for k, v := range m.months[uid] {
		if k < month {
			out = append(out, v)
		}
	}
	return out, nil
}
func (m *memStore) FirstMonth(_ context.Context, uid int64) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	first := ""
	for k := range m.months[uid] {
		if first == "" || k < first {
			first = k
		}
	}
	return first, nil
}
func (m *memStore) ListGoals(_ context.Context, uid int64) ([]store.Goal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.goals[uid], nil
}
func (m *memStore) AddGoal(_ context.Context, uid int64, g store.Goal) (store.Goal, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.goals[uid] {
		if e.ID == g.ID {
			return e, false, nil
		}
	}
	m.goals[uid] = append(m.goals[uid], g)
	return g, true, nil
}
func (m *memStore) ListUserIDs(context.Context) ([]int64, error) { return nil, nil }
func (m *memStore) Close() error                                 { return nil }

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
}

func newEnv(allowed ...int64) testEnv {
	cfg := &config.Config{BotToken: botToken, Salary: 16000, AllowedUserIDs: allowed, WebAppURL: "https://app.example"}
	return testEnv{h: New(newMem(), cfg, nil).Handler(), cfg: cfg}
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

	code, b := e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuid1+`","kind":"expense","amount":35000,"category":"Транспорт","note":"такси"}`)
	if code != 201 {
		t.Fatalf("create: %d %v", code, b)
	}
	if rem := b["summary"].(map[string]any)["remaining"].(float64); rem != -35000 {
		t.Errorf("remaining = %v", rem)
	}
	// idempotent retry
	if code, _ := e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuid1+`","kind":"expense","amount":35000,"category":"Транспорт"}`); code != 200 {
		t.Errorf("retry: %d, want 200", code)
	}
	_, s := e.do(t, "GET", "/api/summary", auth, "")
	if s["expense"].(float64) != 35000 {
		t.Errorf("duplicate counted: %v", s["expense"])
	}

	cases := []struct{ body, field string }{
		{`{"clientId":"x","kind":"expense","amount":1,"category":"Еда"}`, "clientId"},
		{`{"clientId":"` + uuid2 + `","kind":"refund","amount":1}`, "kind"},
		{`{"clientId":"` + uuid2 + `","kind":"expense","amount":0,"category":"Еда"}`, "amount"},
		{`{"clientId":"` + uuid2 + `","kind":"expense","amount":5,"category":"Нет такой"}`, "category"},
		{`{"clientId":"` + uuid2 + `","kind":"income","amount":5,"category":"Еда"}`, "category"},
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
	e.do(t, "POST", "/api/transactions", auth, `{"clientId":"`+uuid2+`","kind":"savings_deposit","amount":150000}`)
	_, b = e.do(t, "GET", "/api/goals", auth, "")
	if p := b["items"].([]any)[0].(map[string]any)["progress"].(float64); p != 0.5 {
		t.Errorf("progress = %v, want 0.5", p)
	}
}

func TestAdvisorUnavailable(t *testing.T) {
	e := newEnv()
	if code, b := e.do(t, "POST", "/api/advisor/report", initData(42), ""); code != 503 || errCode(b) != "advisor_unavailable" {
		t.Errorf("advisor: %d %v", code, b)
	}
}
