// Package firestore implements store.Store on Cloud Firestore (Native mode).
//
// Layout (see specs/004-telegram-mini-app/data-model.md):
//
//	users/{tgId}                        profile + savingsBalance + creditDebt
//	users/{tgId}/transactions/{txId}    immutable ledger entries
//	users/{tgId}/months/{YYYY-MM}       aggregates, changed only via ledger.Apply
//	users/{tgId}/goals/{goalId}
package firestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"expense-bot/internal/catalog"
	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

const database = "(default)"

// Store is the Firestore-backed store.Store.
type Store struct {
	c *firestore.Client
}

var _ store.Store = (*Store)(nil)

// New connects using service-account JSON. projectID defaults to the credentials' project_id.
func New(ctx context.Context, credentialsJSON []byte, projectID string) (*Store, error) {
	if projectID == "" {
		var creds struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(credentialsJSON, &creds); err != nil {
			return nil, fmt.Errorf("parse credentials: %w", err)
		}
		projectID = creds.ProjectID
	}
	if projectID == "" {
		return nil, errors.New("firestore project id is empty: set FIRESTORE_PROJECT_ID")
	}
	c, err := firestore.NewClientWithDatabase(ctx, projectID, database,
		option.WithAuthCredentialsJSON(option.ServiceAccount, credentialsJSON))
	if err != nil {
		return nil, fmt.Errorf("firestore client: %w", err)
	}
	log.Printf("store: firestore project=%s database=%s", projectID, database)
	return &Store{c: c}, nil
}

func (s *Store) Close() error { return s.c.Close() }

// ── refs ─────────────────────────────────────────────────────────────────────

func (s *Store) user(id int64) *firestore.DocumentRef {
	return s.c.Collection("users").Doc(strconv.FormatInt(id, 10))
}
func (s *Store) txs(id int64) *firestore.CollectionRef    { return s.user(id).Collection("transactions") }
func (s *Store) months(id int64) *firestore.CollectionRef { return s.user(id).Collection("months") }
func (s *Store) goals(id int64) *firestore.CollectionRef  { return s.user(id).Collection("goals") }

func notFound(err error) bool { return status.Code(err) == codes.NotFound }

// ── users ────────────────────────────────────────────────────────────────────

func (s *Store) EnsureUser(ctx context.Context, id int64, firstName, username string) (store.User, bool, error) {
	ref := s.user(id)
	var (
		u       store.User
		created bool
	)
	err := s.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		now := time.Now()
		snap, err := tx.Get(ref)
		if err != nil && !notFound(err) {
			return err
		}
		if snap != nil && snap.Exists() {
			if err := snap.DataTo(&u); err != nil {
				return err
			}
			created = false
			u.LastSeenAt = now
			return tx.Set(ref, map[string]any{"lastSeenAt": now}, firestore.MergeAll)
		}
		u = store.User{
			FirstName:    firstName,
			Username:     strings.TrimPrefix(username, "@"),
			RegisteredAt: now,
			LastSeenAt:   now,
		}
		created = true
		// Merge (not Create): balance fields may already exist from ledger writes.
		// New users start with the 14 default categories (feature 006, FR-018).
		u.Categories, u.CategoriesVersion = catalog.Defaults(now), 1
		return tx.Set(ref, map[string]any{
			"firstName":         u.FirstName,
			"username":          u.Username,
			"registeredAt":      u.RegisteredAt,
			"lastSeenAt":        u.LastSeenAt,
			"categories":        u.Categories,
			"categoriesVersion": u.CategoriesVersion,
		}, firestore.MergeAll)
	})
	if err != nil {
		return store.User{}, false, fmt.Errorf("ensure user %d: %w", id, err)
	}
	u.ID = id
	return u, created, nil
}

func (s *Store) GetUser(ctx context.Context, id int64) (store.User, error) {
	snap, err := s.user(id).Get(ctx)
	if notFound(err) {
		return store.User{ID: id}, nil
	}
	if err != nil {
		return store.User{}, fmt.Errorf("get user %d: %w", id, err)
	}
	var u store.User
	if err := snap.DataTo(&u); err != nil {
		return store.User{}, err
	}
	u.ID = id
	return u, nil
}

func (s *Store) ListUserIDs(ctx context.Context) ([]int64, error) {
	var ids []int64
	it := s.c.Collection("users").DocumentRefs(ctx)
	for {
		ref, err := it.Next()
		if err == iterator.Done {
			return ids, nil
		}
		if err != nil {
			return nil, fmt.Errorf("list users: %w", err)
		}
		if id, err := strconv.ParseInt(ref.ID, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
}

// ── ledger ───────────────────────────────────────────────────────────────────

func (s *Store) AddTransaction(ctx context.Context, userID int64, t store.Transaction) (store.Transaction, bool, error) {
	if t.ID == "" {
		return store.Transaction{}, false, errors.New("transaction id is required")
	}
	userRef := s.user(userID)
	txRef := s.txs(userID).Doc(t.ID)
	monthRef := s.months(userID).Doc(t.Month)

	var (
		stored  store.Transaction
		created bool
	)
	if t.OccurredAt.IsZero() {
		t.OccurredAt = t.CreatedAt
	}
	if t.Version == 0 {
		t.Version = 1
	}
	err := s.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// All reads before writes.
		existing, err := tx.Get(txRef)
		if err != nil && !notFound(err) {
			return err
		}
		if existing != nil && existing.Exists() {
			if err := existing.DataTo(&stored); err != nil {
				return err
			}
			created = false
			return nil
		}
		// Chronological balance rule (constitution II): covers back-dated records too.
		if err := s.checkBalance(ctx, tx, userID, t.Kind, "", &t); err != nil {
			return err
		}

		d := ledger.Apply(t.Kind, t.Category, t.Amount)
		if err := tx.Create(txRef, t); err != nil {
			return err
		}
		if err := tx.Set(monthRef, monthIncrements(t.Month, d.Month), firestore.MergeAll); err != nil {
			return err
		}
		userInc := map[string]any{}
		if d.SavingsBalance != 0 {
			userInc["savingsBalance"] = firestore.Increment(d.SavingsBalance)
		}
		if d.CreditDebt != 0 {
			userInc["creditDebt"] = firestore.Increment(d.CreditDebt)
		}
		if len(userInc) > 0 {
			if err := tx.Set(userRef, userInc, firestore.MergeAll); err != nil {
				return err
			}
		}
		stored, created = t, true
		return nil
	})
	if err != nil {
		var le *store.LimitError
		if errors.As(err, &le) {
			return store.Transaction{}, false, le
		}
		return store.Transaction{}, false, fmt.Errorf("add transaction: %w", err)
	}
	stored.ID = t.ID
	return stored, created, nil
}

// monthIncrements turns a delta into a merge-set payload of Increment transforms.
func monthIncrements(month string, d ledger.Month) map[string]any {
	m := map[string]any{"month": month}
	add := func(field string, v int64) {
		if v != 0 {
			m[field] = firestore.Increment(v)
		}
	}
	add("income", d.Income)
	add("expense", d.Expense)
	add("savingsNet", d.SavingsNet)
	add("creditCharged", d.CreditCharged)
	add("creditRepaid", d.CreditRepaid)
	add("cashNet", d.CashNet)
	if len(d.ByCategory) > 0 {
		sub := map[string]any{}
		for k, v := range d.ByCategory {
			sub[k] = firestore.Increment(v)
		}
		m["byCategory"] = sub
	}
	if len(d.ByCreditCategory) > 0 {
		sub := map[string]any{}
		for k, v := range d.ByCreditCategory {
			sub[k] = firestore.Increment(v)
		}
		m["byCreditCategory"] = sub
	}
	return m
}

func (s *Store) ListTransactions(ctx context.Context, userID int64, month string, limit int) ([]store.Transaction, error) {
	it := s.txs(userID).Where("month", "==", month).OrderBy("createdAt", firestore.Desc).Limit(limit).Documents(ctx)
	defer it.Stop()
	var out []store.Transaction
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("list transactions: %w", err)
		}
		var t store.Transaction
		if err := snap.DataTo(&t); err != nil {
			return nil, err
		}
		t.ID = snap.Ref.ID
		out = append(out, t)
	}
}

// ── months ───────────────────────────────────────────────────────────────────

func (s *Store) GetMonth(ctx context.Context, userID int64, month string) (ledger.Month, error) {
	snap, err := s.months(userID).Doc(month).Get(ctx)
	if notFound(err) {
		return ledger.Month{Month: month}, nil
	}
	if err != nil {
		return ledger.Month{}, fmt.Errorf("get month %s: %w", month, err)
	}
	var m ledger.Month
	if err := snap.DataTo(&m); err != nil {
		return ledger.Month{}, err
	}
	m.Month = month
	return m, nil
}

func (s *Store) MonthsBefore(ctx context.Context, userID int64, month string) ([]ledger.Month, error) {
	return s.queryMonths(ctx, s.months(userID).Where("month", "<", month))
}

func (s *Store) FirstMonth(ctx context.Context, userID int64) (string, error) {
	ms, err := s.queryMonths(ctx, s.months(userID).OrderBy("month", firestore.Asc).Limit(1))
	if err != nil || len(ms) == 0 {
		return "", err
	}
	return ms[0].Month, nil
}

func (s *Store) queryMonths(ctx context.Context, q firestore.Query) ([]ledger.Month, error) {
	it := q.Documents(ctx)
	defer it.Stop()
	var out []ledger.Month
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("query months: %w", err)
		}
		var m ledger.Month
		if err := snap.DataTo(&m); err != nil {
			return nil, err
		}
		m.Month = snap.Ref.ID
		out = append(out, m)
	}
}

// ── goals ────────────────────────────────────────────────────────────────────

func (s *Store) ListGoals(ctx context.Context, userID int64) ([]store.Goal, error) {
	it := s.goals(userID).OrderBy("createdAt", firestore.Asc).Documents(ctx)
	defer it.Stop()
	var out []store.Goal
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("list goals: %w", err)
		}
		var g store.Goal
		if err := snap.DataTo(&g); err != nil {
			return nil, err
		}
		g.ID = snap.Ref.ID
		out = append(out, g)
	}
}

// GoalSavings sums savings deposits by goalId. Progress is read-time, so editing or deleting a
// deposit needs no goal bookkeeping.
func (s *Store) GoalSavings(ctx context.Context, userID int64) (map[string]int64, error) {
	it := s.txs(userID).Where("kind", "==", string(ledger.KindSavingsDeposit)).Select("goalId", "amount").Documents(ctx)
	defer it.Stop()
	out := map[string]int64{}
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("goal savings: %w", err)
		}
		var t store.Transaction
		if err := snap.DataTo(&t); err != nil {
			return nil, err
		}
		if t.GoalID != "" {
			out[t.GoalID] += t.Amount
		}
	}
}

func (s *Store) AddGoal(ctx context.Context, userID int64, g store.Goal) (store.Goal, bool, error) {
	if g.ID == "" {
		return store.Goal{}, false, errors.New("goal id is required")
	}
	ref := s.goals(userID).Doc(g.ID)
	_, err := ref.Create(ctx, g)
	if status.Code(err) == codes.AlreadyExists {
		snap, err := ref.Get(ctx)
		if err != nil {
			return store.Goal{}, false, fmt.Errorf("get goal: %w", err)
		}
		var existing store.Goal
		if err := snap.DataTo(&existing); err != nil {
			return store.Goal{}, false, err
		}
		existing.ID = g.ID
		return existing, false, nil
	}
	if err != nil {
		return store.Goal{}, false, fmt.Errorf("add goal: %w", err)
	}
	return g, true, nil
}
