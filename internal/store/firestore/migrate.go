package firestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"expense-bot/internal/catalog"
	"expense-bot/internal/category"
	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

// Migration-only helpers (cmd/migrate). They write without ledger validation, so the
// aggregates must be rebuilt afterwards with RebuildAggregates.

// UpsertUserRaw writes profile fields; registeredAt is kept if the user already exists.
func (s *Store) UpsertUserRaw(ctx context.Context, u store.User) error {
	ref := s.user(u.ID)
	snap, err := ref.Get(ctx)
	if err != nil && !notFound(err) {
		return err
	}
	data := map[string]any{"firstName": u.FirstName, "username": u.Username}
	if snap == nil || !snap.Exists() {
		reg := u.RegisteredAt
		if reg.IsZero() {
			reg = time.Now()
		}
		data["registeredAt"] = reg
		data["lastSeenAt"] = reg
	}
	_, err = ref.Set(ctx, data, firestore.MergeAll)
	return err
}

// ShouldSkip reports whether the migration must leave a record alone (FR-011): the user
// edited it (editedAt set) or deleted it (tombstone).
func ShouldSkip(existing *store.Transaction, tombstoned bool) bool {
	return tombstoned || (existing != nil && existing.EditedAt != nil)
}

// UpsertTransactionsRaw overwrites migrated transactions by ID (idempotent), skipping records
// the user edited or deleted. It returns how many were skipped for each reason.
func (s *Store) UpsertTransactionsRaw(ctx context.Context, userID int64, txs []store.Transaction) (edited, deleted int, err error) {
	tomb, err := s.tombstones(ctx, userID)
	if err != nil {
		return 0, 0, err
	}
	refs := make([]*firestore.DocumentRef, len(txs))
	for i, t := range txs {
		refs[i] = s.txs(userID).Doc(t.ID)
	}
	snaps, err := s.c.GetAll(ctx, refs)
	if err != nil {
		return 0, 0, fmt.Errorf("read existing: %w", err)
	}

	bw := s.c.BulkWriter(ctx)
	jobs := make([]*firestore.BulkWriterJob, 0, len(txs))
	for i, t := range txs {
		var existing *store.Transaction
		if snaps[i].Exists() {
			var e store.Transaction
			if err := snaps[i].DataTo(&e); err != nil {
				bw.End()
				return 0, 0, err
			}
			existing = &e
		}
		if ShouldSkip(existing, tomb[t.ID]) {
			if tomb[t.ID] {
				deleted++
			} else {
				edited++
			}
			continue
		}
		if t.OccurredAt.IsZero() {
			t.OccurredAt = t.CreatedAt
		}
		t.Version = 1
		j, err := bw.Set(refs[i], t)
		if err != nil {
			bw.End()
			return 0, 0, err
		}
		jobs = append(jobs, j)
	}
	bw.End()
	for _, j := range jobs {
		if _, err := j.Results(); err != nil {
			return 0, 0, fmt.Errorf("upsert transaction: %w", err)
		}
	}
	return edited, deleted, nil
}

func (s *Store) tombstones(ctx context.Context, userID int64) (map[string]bool, error) {
	out := map[string]bool{}
	it := s.user(userID).Collection("tombstones").DocumentRefs(ctx)
	for {
		ref, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("list tombstones: %w", err)
		}
		out[ref.ID] = true
	}
}

// Backfill005 sets occurredAt = createdAt and version = 1 where missing (feature 005 rollout).
// Nothing else is touched; idempotent.
func (s *Store) Backfill005(ctx context.Context, userID int64) (txs, goals int, err error) {
	bw := s.c.BulkWriter(ctx)
	var jobs []*firestore.BulkWriterJob
	add := func(ref *firestore.DocumentRef, data map[string]any) error {
		j, err := bw.Set(ref, data, firestore.MergeAll)
		if err == nil {
			jobs = append(jobs, j)
		}
		return err
	}
	it := s.txs(userID).Documents(ctx)
	for {
		snap, e := it.Next()
		if e == iterator.Done {
			break
		}
		if e != nil {
			it.Stop()
			bw.End()
			return 0, 0, e
		}
		data := snap.Data()
		upd := map[string]any{}
		if _, ok := data["occurredAt"]; !ok {
			upd["occurredAt"] = data["createdAt"]
		}
		if _, ok := data["version"]; !ok {
			upd["version"] = int64(1)
		}
		if len(upd) > 0 {
			if e := add(snap.Ref, upd); e != nil {
				bw.End()
				return 0, 0, e
			}
			txs++
		}
	}
	git := s.goals(userID).Documents(ctx)
	for {
		snap, e := git.Next()
		if e == iterator.Done {
			break
		}
		if e != nil {
			git.Stop()
			bw.End()
			return 0, 0, e
		}
		if _, ok := snap.Data()["version"]; !ok {
			if e := add(snap.Ref, map[string]any{"version": int64(1)}); e != nil {
				bw.End()
				return 0, 0, e
			}
			goals++
		}
	}
	bw.End()
	for _, j := range jobs {
		if _, e := j.Results(); e != nil {
			return 0, 0, fmt.Errorf("backfill: %w", e)
		}
	}
	return txs, goals, nil
}

// DeleteStaleMigrated removes migration-sourced transactions and migration goals whose IDs
// are not in keep. Sheet rows that were edited, moved or deleted get new IDs (row index based),
// so without this a re-run would leave stale duplicates. App-created data is never touched.
func (s *Store) DeleteStaleMigrated(ctx context.Context, userID int64, keepTx, keepGoals map[string]bool) (int, error) {
	bw := s.c.BulkWriter(ctx)
	var jobs []*firestore.BulkWriterJob
	txs, err := s.AllTransactions(ctx, userID)
	if err != nil {
		bw.End()
		return 0, err
	}
	for _, t := range txs {
		if t.Source == "migration" && !keepTx[t.ID] && t.EditedAt == nil {
			j, err := bw.Delete(s.txs(userID).Doc(t.ID))
			if err != nil {
				bw.End()
				return 0, err
			}
			jobs = append(jobs, j)
		}
	}
	goals, err := s.ListGoals(ctx, userID)
	if err != nil {
		bw.End()
		return 0, err
	}
	for _, g := range goals {
		if strings.HasPrefix(g.ID, "m_") && !keepGoals[g.ID] {
			j, err := bw.Delete(s.goals(userID).Doc(g.ID))
			if err != nil {
				bw.End()
				return 0, err
			}
			jobs = append(jobs, j)
		}
	}
	bw.End()
	for _, j := range jobs {
		if _, err := j.Results(); err != nil {
			return 0, fmt.Errorf("delete stale: %w", err)
		}
	}
	return len(jobs), nil
}

// UpsertGoalsRaw overwrites goals by ID (idempotent).
func (s *Store) UpsertGoalsRaw(ctx context.Context, userID int64, goals []store.Goal) error {
	for _, g := range goals {
		if _, err := s.goals(userID).Doc(g.ID).Set(ctx, g); err != nil {
			return fmt.Errorf("upsert goal: %w", err)
		}
	}
	return nil
}

// AllTransactions returns every transaction of the user.
func (s *Store) AllTransactions(ctx context.Context, userID int64) ([]store.Transaction, error) {
	it := s.txs(userID).Documents(ctx)
	defer it.Stop()
	var out []store.Transaction
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read transactions: %w", err)
		}
		var t store.Transaction
		if err := snap.DataTo(&t); err != nil {
			return nil, err
		}
		t.ID = snap.Ref.ID
		out = append(out, t)
	}
}

// Fold recomputes month aggregates and balances from transactions.
func Fold(txs []store.Transaction) (months map[string]ledger.Month, savings, debt int64) {
	months = map[string]ledger.Month{}
	for _, t := range txs {
		d := ledger.Apply(t.Kind, t.Category, t.Amount)
		m := months[t.Month]
		m.Month = t.Month
		m.Add(d.Month)
		months[t.Month] = m
		savings += d.SavingsBalance
		debt += d.CreditDebt
	}
	return months, savings, debt
}

// RebuildAggregates recomputes all month docs and user balances from the user's transactions
// (overwrite, never increment). It returns the rebuilt months and the transaction count.
func (s *Store) RebuildAggregates(ctx context.Context, userID int64) (map[string]ledger.Month, int, error) {
	txs, err := s.AllTransactions(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	months, savings, debt := Fold(txs)
	count := len(txs)

	bw := s.c.BulkWriter(ctx)
	var jobs []*firestore.BulkWriterJob
	// Delete month docs that no longer have transactions.
	existing := s.months(userID).DocumentRefs(ctx)
	for {
		ref, err := existing.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			bw.End()
			return nil, 0, fmt.Errorf("list months: %w", err)
		}
		if _, ok := months[ref.ID]; !ok {
			j, err := bw.Delete(ref)
			if err != nil {
				bw.End()
				return nil, 0, err
			}
			jobs = append(jobs, j)
		}
	}
	for key, m := range months {
		j, err := bw.Set(s.months(userID).Doc(key), m)
		if err != nil {
			bw.End()
			return nil, 0, err
		}
		jobs = append(jobs, j)
	}
	j, err := bw.Set(s.user(userID), map[string]any{"savingsBalance": savings, "creditDebt": debt}, firestore.MergeAll)
	if err != nil {
		bw.End()
		return nil, 0, err
	}
	jobs = append(jobs, j)
	bw.End()
	for _, j := range jobs {
		if _, err := j.Results(); err != nil {
			return nil, 0, fmt.Errorf("write aggregates: %w", err)
		}
	}
	return months, count, nil
}

// ConvertReport summarises one user's 006 conversion.
type ConvertReport struct {
	Created    []string // category IDs added (defaults and legacy)
	Legacy     []string // legacy names that became hidden categories
	Rewritten  int      // records whose category value changed name → ID
	Mismatches []string
}

// Convert006 switches a user from category names to per-user category IDs (feature 006,
// research R5): builds the category map (keeping existing entries), rewrites record
// categories, drops 005 profile limits, rebuilds aggregates and verifies per-category totals
// against the before-snapshot. Records keep version/editedAt (schema conversion, not an edit).
func (s *Store) Convert006(ctx context.Context, userID int64, dryRun bool) (ConvertReport, error) {
	var rep ConvertReport
	before, err := s.queryMonths(ctx, s.months(userID).Query)
	if err != nil {
		return rep, err
	}
	txs, err := s.AllTransactions(ctx, userID)
	if err != nil {
		return rep, err
	}
	snap, err := s.user(userID).Get(ctx)
	if err != nil && !notFound(err) {
		return rep, err
	}
	var u store.User
	var limits005 map[string]int64
	if snap != nil && snap.Exists() {
		if err := snap.DataTo(&u); err != nil {
			return rep, err
		}
		if raw, ok := snap.Data()["limits"].(map[string]any); ok {
			limits005 = map[string]int64{}
			for k, v := range raw {
				if n, ok := v.(int64); ok {
					limits005[k] = n
				}
			}
		}
	}

	values := make([]string, 0, len(txs))
	for _, t := range txs {
		values = append(values, t.Category)
	}
	list, mapping := catalog.Convert(u.Categories, values, limits005, time.Now().UTC())
	for id := range list {
		if _, ok := u.Categories[id]; !ok {
			rep.Created = append(rep.Created, id)
		}
	}
	for name, id := range mapping {
		if catalog.IsID(id) && strings.HasPrefix(id, catalog.IDPrefix+"l") {
			rep.Legacy = append(rep.Legacy, name)
		}
	}
	for _, t := range txs {
		if _, ok := mapping[t.Category]; ok {
			rep.Rewritten++
		}
	}
	if dryRun {
		return rep, nil
	}

	bw := s.c.BulkWriter(ctx)
	var jobs []*firestore.BulkWriterJob
	for _, t := range txs {
		id, ok := mapping[t.Category]
		if !ok {
			continue
		}
		j, err := bw.Set(s.txs(userID).Doc(t.ID), map[string]any{"category": id}, firestore.MergeAll)
		if err != nil {
			bw.End()
			return rep, err
		}
		jobs = append(jobs, j)
	}
	bw.End()
	for _, j := range jobs {
		if _, err := j.Results(); err != nil {
			return rep, fmt.Errorf("rewrite category: %w", err)
		}
	}
	ver := u.CategoriesVersion
	if ver == 0 {
		ver = 1
	}
	if _, err := s.user(userID).Set(ctx, map[string]any{
		"categories": list, "categoriesVersion": ver + 1, "limits": firestore.Delete,
	}, firestore.Merge([]string{"categories"}, []string{"categoriesVersion"}, []string{"limits"})); err != nil {
		return rep, fmt.Errorf("write categories: %w", err)
	}
	if _, _, err := s.RebuildAggregates(ctx, userID); err != nil {
		return rep, err
	}

	// Verify: rebuilt aggregates == before-snapshot re-keyed through the mapping.
	after, err := s.queryMonths(ctx, s.months(userID).Query)
	if err != nil {
		return rep, err
	}
	afterBy := map[string]ledger.Month{}
	for _, m := range after {
		afterBy[m.Month] = m
	}
	for _, b := range before {
		a := afterBy[b.Month]
		check := func(field string, want, got int64) {
			if want != got {
				rep.Mismatches = append(rep.Mismatches, fmt.Sprintf("month=%s %s: before=%d after=%d", b.Month, field, want, got))
			}
		}
		check("income", b.Income, a.Income)
		check("expense", b.Expense, a.Expense)
		check("savingsNet", b.SavingsNet, a.SavingsNet)
		check("creditCharged", b.CreditCharged, a.CreditCharged)
		check("creditRepaid", b.CreditRepaid, a.CreditRepaid)
		check("cashNet", b.CashNet, a.CashNet)
		for k, v := range catalog.MapTotals(b.ByCategory, mapping) {
			check("byCategory."+k, v, a.ByCategory[k])
		}
		for k, v := range catalog.MapTotals(b.ByCreditCategory, mapping) {
			check("byCreditCategory."+k, v, a.ByCreditCategory[k])
		}
	}
	// Default limits: the 005 effective limit must equal the new category limit.
	for _, c := range category.All() {
		want := int64(c.Limit) * ledger.PerSomoni
		if v, ok := limits005[c.Name]; ok {
			want = v
		}
		if e, ok := list[catalog.DefaultID(c.Key)]; ok && e.Limit != want {
			if _, existed := u.Categories[catalog.DefaultID(c.Key)]; !existed {
				rep.Mismatches = append(rep.Mismatches, fmt.Sprintf("limit %s: before=%d after=%d", c.Key, want, e.Limit))
			}
		}
	}
	return rep, nil
}
