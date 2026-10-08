package firestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

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

// UpsertTransactionsRaw overwrites transactions by ID (idempotent).
func (s *Store) UpsertTransactionsRaw(ctx context.Context, userID int64, txs []store.Transaction) error {
	bw := s.c.BulkWriter(ctx)
	jobs := make([]*firestore.BulkWriterJob, 0, len(txs))
	for _, t := range txs {
		j, err := bw.Set(s.txs(userID).Doc(t.ID), t)
		if err != nil {
			bw.End()
			return err
		}
		jobs = append(jobs, j)
	}
	bw.End()
	for _, j := range jobs {
		if _, err := j.Results(); err != nil {
			return fmt.Errorf("upsert transaction: %w", err)
		}
	}
	return nil
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
		if t.Source == "migration" && !keepTx[t.ID] {
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
