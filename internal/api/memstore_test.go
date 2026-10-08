package api

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"expense-bot/internal/catalog"
	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

// memStore is an in-memory store.Store with the same ledger semantics as the Firestore
// implementation (Apply/Revert, CheckRunning, versioning, tombstones). Reference for API tests.
type memStore struct {
	sumErr     error                             // injected SumTransactions failure (e.g. missing Firestore index)
	payouts    map[int64]map[string]store.Payout // key month_kind
	mu         sync.Mutex
	users      map[int64]*store.User
	txs        map[int64]map[string]store.Transaction
	months     map[int64]map[string]ledger.Month
	goals      map[int64]map[string]store.Goal
	tombstones map[int64]map[string]bool
}

func newMem() *memStore {
	return &memStore{users: map[int64]*store.User{}, txs: map[int64]map[string]store.Transaction{},
		months: map[int64]map[string]ledger.Month{}, goals: map[int64]map[string]store.Goal{},
		tombstones: map[int64]map[string]bool{}, payouts: map[int64]map[string]store.Payout{}}
}

func (m *memStore) user(id int64) *store.User {
	u, ok := m.users[id]
	if !ok {
		u = &store.User{ID: id}
		m.users[id] = u
	}
	if m.txs[id] == nil {
		m.txs[id], m.months[id], m.goals[id], m.tombstones[id] =
			map[string]store.Transaction{}, map[string]ledger.Month{}, map[string]store.Goal{}, map[string]bool{}
	}
	return u
}

func (m *memStore) EnsureUser(_ context.Context, id int64, fn, un string) (store.User, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[id]; ok && !u.RegisteredAt.IsZero() {
		return *u, false, nil
	}
	u := m.user(id)
	u.FirstName, u.Username, u.RegisteredAt = fn, un, time.Now()
	u.Categories, u.CategoriesVersion = catalog.Defaults(u.RegisteredAt), 1
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

// apply moves aggregates by d in month.
func (m *memStore) apply(uid int64, month string, d ledger.Delta) {
	mo := m.months[uid][month]
	mo.Month = month
	mo.Add(d.Month)
	m.months[uid][month] = mo
	u := m.users[uid]
	u.SavingsBalance += d.SavingsBalance
	u.CreditDebt += d.CreditDebt
}

// check runs the chronological balance rule like firestore.checkBalance.
func (m *memStore) check(uid int64, k ledger.Kind, replaceID string, next *store.Transaction) error {
	var delta func(ledger.Kind, int64) (int64, bool)
	var errKind error
	if _, ok := ledger.SavingsDelta(k, 0); ok {
		delta, errKind = ledger.SavingsDelta, store.ErrInsufficientSavings
	} else if _, ok := ledger.DebtDelta(k, 0); ok {
		delta, errKind = ledger.DebtDelta, store.ErrExceedsDebt
	} else {
		return nil
	}
	ev := func(t store.Transaction) (ledger.Event, bool) {
		d, ok := delta(t.Kind, t.Amount)
		return ledger.Event{At: t.When(), Seq: t.CreatedAt, Delta: d}, ok
	}
	var before, after []ledger.Event
	var cur int64
	for id, t := range m.txs[uid] {
		e, ok := ev(t)
		if !ok {
			continue
		}
		before = append(before, e)
		cur += e.Delta
		if id != replaceID {
			after = append(after, e)
		}
	}
	if next != nil {
		e, _ := ev(*next)
		after = append(after, e)
	}
	if v := ledger.CheckRunning(before, after); v != nil {
		return &store.LimitError{Err: errKind, Available: cur, At: v.At, Balance: v.Balance}
	}
	return nil
}

func (m *memStore) AddTransaction(_ context.Context, uid int64, t store.Transaction) (store.Transaction, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user(uid)
	if old, ok := m.txs[uid][t.ID]; ok {
		return old, false, nil
	}
	if t.OccurredAt.IsZero() {
		t.OccurredAt = t.CreatedAt
	}
	if t.Version == 0 {
		t.Version = 1
	}
	if err := m.check(uid, t.Kind, "", &t); err != nil {
		return store.Transaction{}, false, err
	}
	m.txs[uid][t.ID] = t
	m.apply(uid, t.Month, ledger.Apply(t.Kind, t.Category, t.Amount))
	return t, true, nil
}

func (m *memStore) GetTransaction(_ context.Context, uid int64, id string) (store.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.txs[uid][id]
	if !ok {
		return store.Transaction{}, store.ErrNotFound
	}
	return t, nil
}

func (m *memStore) UpdateTransaction(_ context.Context, uid int64, id string, p store.TransactionPatch) (store.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user(uid)
	cur, ok := m.txs[uid][id]
	if !ok {
		return store.Transaction{}, store.ErrNotFound
	}
	if p.RequestID != "" && cur.LastRequestID == p.RequestID {
		return cur, nil
	}
	if p.Version != cur.Version {
		return store.Transaction{}, &store.ConflictError{Current: cur}
	}
	nw := cur
	if p.Amount != nil {
		nw.Amount = *p.Amount
	}
	if p.Category != nil {
		nw.Category = *p.Category
	}
	if p.Note != nil {
		nw.Note = *p.Note
	}
	if p.Date != nil {
		t := cur.When().In(ledger.Location)
		d := p.Date.In(ledger.Location)
		nw.OccurredAt = time.Date(d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), ledger.Location)
	}
	nw.Month = ledger.MonthKey(nw.When())
	if err := m.check(uid, cur.Kind, id, &nw); err != nil {
		return store.Transaction{}, err
	}
	m.apply(uid, cur.Month, ledger.Revert(cur.Kind, cur.Category, cur.Amount))
	m.apply(uid, nw.Month, ledger.Apply(nw.Kind, nw.Category, nw.Amount))
	now := time.Now()
	nw.Version, nw.EditedAt, nw.LastRequestID = cur.Version+1, &now, p.RequestID
	m.txs[uid][id] = nw
	return nw, nil
}

func (m *memStore) DeleteTransaction(_ context.Context, uid int64, id string, ver int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user(uid)
	cur, ok := m.txs[uid][id]
	if !ok {
		return false, nil
	}
	if ver != cur.Version {
		return false, &store.ConflictError{Current: cur}
	}
	if err := m.check(uid, cur.Kind, id, nil); err != nil {
		return false, err
	}
	m.apply(uid, cur.Month, ledger.Revert(cur.Kind, cur.Category, cur.Amount))
	delete(m.txs[uid], id)
	if strings.HasPrefix(id, "m_") {
		m.tombstones[uid][id] = true
	}
	return true, nil
}

// matching returns the user's records matching f, newest first.
func (m *memStore) matching(uid int64, f store.HistoryFilter) []store.Transaction {
	var out []store.Transaction
	for _, t := range m.txs[uid] {
		if len(f.Kinds) > 0 {
			ok := false
			for _, k := range f.Kinds {
				ok = ok || t.Kind == k
			}
			if !ok {
				continue
			}
		}
		if f.Category != "" && t.Category != f.Category {
			continue
		}
		if f.Month != "" && ledger.MonthKey(t.When()) != f.Month {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].When().Equal(out[j].When()) {
			return out[i].When().After(out[j].When())
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (m *memStore) QueryTransactions(_ context.Context, uid int64, f store.HistoryFilter) ([]store.Transaction, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.matching(uid, f)
	start := 0
	if f.Cursor != "" {
		n, err := strconv.Atoi(f.Cursor)
		if err != nil {
			return nil, "", errInvalidCursor
		}
		start = n
	}
	needle := strings.ToLower(f.Q)
	var out []store.Transaction
	i := start
	for ; i < len(all) && len(out) < f.Limit; i++ {
		if needle == "" || strings.Contains(strings.ToLower(all[i].Note), needle) {
			out = append(out, all[i])
		}
	}
	next := ""
	if i < len(all) {
		next = strconv.Itoa(i)
	}
	return out, next, nil
}

type cursorErr struct{}

func (cursorErr) Error() string { return "invalid cursor" }

var errInvalidCursor = cursorErr{}

func (m *memStore) SumTransactions(_ context.Context, uid int64, f store.HistoryFilter) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sumErr != nil {
		return 0, m.sumErr
	}
	var sum int64
	for _, t := range m.matching(uid, f) {
		sum += t.Amount
	}
	return sum, nil
}

func (m *memStore) ListTransactions(ctx context.Context, uid int64, month string, limit int) ([]store.Transaction, error) {
	items, _, err := m.QueryTransactions(ctx, uid, store.HistoryFilter{Month: month, Limit: limit})
	return items, err
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

func (m *memStore) ListMonths(_ context.Context, uid int64, from, to string) ([]ledger.Month, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ledger.Month
	for k, v := range m.months[uid] {
		if (from != "" && k < from) || (to != "" && k > to) {
			continue
		}
		if v.Income != 0 || v.Expense != 0 || v.SavingsNet != 0 || v.CreditCharged != 0 || v.CreditRepaid != 0 {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month > out[j].Month })
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
	var out []store.Goal
	for _, g := range m.goals[uid] {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *memStore) AddGoal(_ context.Context, uid int64, g store.Goal) (store.Goal, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user(uid)
	if e, ok := m.goals[uid][g.ID]; ok {
		return e, false, nil
	}
	if g.Version == 0 {
		g.Version = 1
	}
	m.goals[uid][g.ID] = g
	return g, true, nil
}

func (m *memStore) UpdateGoal(_ context.Context, uid int64, id string, p store.GoalPatch) (store.Goal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user(uid)
	cur, ok := m.goals[uid][id]
	if !ok {
		return store.Goal{}, store.ErrNotFound
	}
	if p.RequestID != "" && cur.LastRequestID == p.RequestID {
		return cur, nil
	}
	if p.Version != cur.Version {
		return store.Goal{}, &store.ConflictError{Current: cur}
	}
	nw := cur
	if p.Name != nil {
		nw.Name = *p.Name
	}
	if p.Target != nil {
		nw.Target = *p.Target
	}
	if p.Quarter != nil {
		nw.Quarter = *p.Quarter
	}
	if p.Note != nil {
		nw.Note = *p.Note
	}
	if p.Status != nil {
		nw.Status = *p.Status
	}
	now := time.Now()
	nw.Version, nw.UpdatedAt, nw.LastRequestID = cur.Version+1, &now, p.RequestID
	m.goals[uid][id] = nw
	return nw, nil
}

func (m *memStore) DeleteGoal(_ context.Context, uid int64, id string, ver int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user(uid)
	cur, ok := m.goals[uid][id]
	if !ok {
		return nil
	}
	if ver != cur.Version {
		return &store.ConflictError{Current: cur}
	}
	delete(m.goals[uid], id)
	return nil
}

func (m *memStore) UpdateProfile(_ context.Context, uid int64, p store.ProfilePatch) (store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u := m.user(uid)
	if p.DisplayName.Set {
		u.DisplayName = p.DisplayName.Value
	}
	if p.Salary.Set {
		u.Salary = p.Salary.Value
	}
	if p.SalaryMode != nil {
		u.SalaryMode = *p.SalaryMode
	}
	if p.Advance.Set {
		u.Advance = p.Advance.Value
	}
	if p.SalaryReminders != nil {
		u.SalaryRemindersOff = !*p.SalaryReminders
	}
	now := time.Now()
	u.ProfileUpdatedAt = &now
	return *u, nil
}

func (m *memStore) UpdateCategories(_ context.Context, uid int64, version int64, fn func(catalog.List) error) (catalog.List, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u := m.user(uid)
	list := u.Categories.Clone()
	if len(list) == 0 {
		list = catalog.Defaults(time.Now())
	}
	cur := u.CategoriesVersion
	if cur == 0 {
		cur = 1
	}
	if version != 0 && version != cur {
		return nil, 0, &store.ConflictError{Current: store.CategoryState{List: u.CategoryList(), Version: cur}}
	}
	if err := fn(list); err != nil {
		return nil, 0, err
	}
	u.Categories, u.CategoriesVersion = list, cur+1
	return list, cur + 1, nil
}

func (m *memStore) GetPayouts(_ context.Context, uid int64, month string) (map[string]store.Payout, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]store.Payout{}
	for _, p := range m.payouts[uid] {
		if p.Month == month {
			out[p.Kind] = p
		}
	}
	return out, nil
}

func (m *memStore) DismissPayout(_ context.Context, uid int64, month, kind string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.payouts[uid] == nil {
		m.payouts[uid] = map[string]store.Payout{}
	}
	p := m.payouts[uid][month+"_"+kind]
	now := time.Now()
	p.Month, p.Kind, p.DismissedAt = month, kind, &now
	m.payouts[uid][month+"_"+kind] = p
	return nil
}

func (m *memStore) ClaimReminder(_ context.Context, uid int64, month, kind, txID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.payouts[uid] == nil {
		m.payouts[uid] = map[string]store.Payout{}
	}
	p := m.payouts[uid][month+"_"+kind]
	if p.ReminderSentAt != nil || p.DismissedAt != nil {
		return false, nil
	}
	if _, ok := m.txs[uid][txID]; ok {
		return false, nil
	}
	now := time.Now()
	p.Month, p.Kind, p.ReminderSentAt = month, kind, &now
	m.payouts[uid][month+"_"+kind] = p
	return true, nil
}

func (m *memStore) ListUserIDs(context.Context) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []int64
	for id := range m.users {
		ids = append(ids, id)
	}
	return ids, nil
}
func (m *memStore) Close() error { return nil }

// seed inserts a record directly (bypassing validation) with a given business time.
func (m *memStore) seed(uid int64, id string, k ledger.Kind, cat string, amount int64, note string, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user(uid)
	t := store.Transaction{ID: id, Kind: k, Category: cat, Amount: amount, Note: note,
		Month: ledger.MonthKey(at), Source: "app", OccurredAt: at, CreatedAt: at, Version: 1}
	m.txs[uid][id] = t
	m.apply(uid, t.Month, ledger.Apply(k, cat, amount))
}
