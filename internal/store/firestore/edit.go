package firestore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"

	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

// version treats records written before versioning (0) as version 1.
func version(v int64) int64 {
	if v == 0 {
		return 1
	}
	return v
}

// withDate keeps t's time of day (Dushanbe) on the calendar day of date.
func withDate(t, date time.Time) time.Time {
	t = t.In(ledger.Location)
	d := date.In(ledger.Location)
	return time.Date(d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), ledger.Location)
}

// userIncrements turns balance deltas into a merge-set payload.
func userIncrements(d ledger.Delta) map[string]any {
	inc := map[string]any{}
	if d.SavingsBalance != 0 {
		inc["savingsBalance"] = firestore.Increment(d.SavingsBalance)
	}
	if d.CreditDebt != 0 {
		inc["creditDebt"] = firestore.Increment(d.CreditDebt)
	}
	return inc
}

// applyDeltas writes month and user increments for old (reverted) and new (applied) values.
func (s *Store) applyDeltas(tx *firestore.Transaction, userID int64, oldMonth string, dOld ledger.Delta,
	newMonth string, dNew *ledger.Delta) error {
	if dNew != nil && newMonth == oldMonth {
		sum := dOld
		sum.Add(*dNew)
		dOld, dNew = sum, nil
	}
	if err := tx.Set(s.months(userID).Doc(oldMonth), monthIncrements(oldMonth, dOld.Month), firestore.MergeAll); err != nil {
		return err
	}
	bal := dOld
	if dNew != nil {
		if err := tx.Set(s.months(userID).Doc(newMonth), monthIncrements(newMonth, dNew.Month), firestore.MergeAll); err != nil {
			return err
		}
		bal.Add(ledger.Delta{SavingsBalance: dNew.SavingsBalance, CreditDebt: dNew.CreditDebt})
	}
	if inc := userIncrements(bal); len(inc) > 0 {
		return tx.Set(s.user(userID), inc, firestore.MergeAll)
	}
	return nil
}

// GetTransaction returns one record or store.ErrNotFound.
func (s *Store) GetTransaction(ctx context.Context, userID int64, id string) (store.Transaction, error) {
	snap, err := s.txs(userID).Doc(id).Get(ctx)
	if notFound(err) {
		return store.Transaction{}, store.ErrNotFound
	}
	if err != nil {
		return store.Transaction{}, fmt.Errorf("get transaction: %w", err)
	}
	var t store.Transaction
	if err := snap.DataTo(&t); err != nil {
		return store.Transaction{}, err
	}
	t.ID = id
	return t, nil
}

// UpdateTransaction edits a record in place: Revert(old) + Apply(new) in one transaction.
func (s *Store) UpdateTransaction(ctx context.Context, userID int64, id string, p store.TransactionPatch) (store.Transaction, error) {
	ref := s.txs(userID).Doc(id)
	var result store.Transaction
	err := s.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if notFound(err) {
			return store.ErrNotFound
		}
		if err != nil {
			return err
		}
		var cur store.Transaction
		if err := snap.DataTo(&cur); err != nil {
			return err
		}
		cur.ID = id
		cur.Version = version(cur.Version)
		if p.RequestID != "" && cur.LastRequestID == p.RequestID {
			result = cur // retry of an already applied edit
			return nil
		}
		if p.Version != cur.Version {
			return &store.ConflictError{Current: cur}
		}

		nw := cur
		nw.OccurredAt = cur.When()
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
			nw.OccurredAt = withDate(nw.OccurredAt, *p.Date)
		}
		nw.Month = ledger.MonthKey(nw.OccurredAt)

		if err := s.checkBalance(ctx, tx, userID, cur.Kind, id, &nw); err != nil {
			return err
		}

		dOld := ledger.Revert(cur.Kind, cur.Category, cur.Amount)
		dNew := ledger.Apply(nw.Kind, nw.Category, nw.Amount)
		if err := s.applyDeltas(tx, userID, cur.Month, dOld, nw.Month, &dNew); err != nil {
			return err
		}
		now := time.Now().UTC()
		nw.Version = cur.Version + 1
		nw.EditedAt = &now
		nw.LastRequestID = p.RequestID
		if err := tx.Set(ref, nw); err != nil {
			return err
		}
		result = nw
		return nil
	})
	if err != nil {
		return store.Transaction{}, wrapTxErr("update transaction", err)
	}
	return result, nil
}

// DeleteTransaction reverts and deletes a record; migrated records leave a tombstone.
func (s *Store) DeleteTransaction(ctx context.Context, userID int64, id string, ver int64) (bool, error) {
	ref := s.txs(userID).Doc(id)
	deleted := false
	err := s.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		deleted = false
		snap, err := tx.Get(ref)
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		var cur store.Transaction
		if err := snap.DataTo(&cur); err != nil {
			return err
		}
		cur.ID = id
		cur.Version = version(cur.Version)
		if ver != cur.Version {
			return &store.ConflictError{Current: cur}
		}
		if err := s.checkBalance(ctx, tx, userID, cur.Kind, id, nil); err != nil {
			return err
		}
		if err := s.applyDeltas(tx, userID, cur.Month, ledger.Revert(cur.Kind, cur.Category, cur.Amount), "", nil); err != nil {
			return err
		}
		if err := tx.Delete(ref); err != nil {
			return err
		}
		if strings.HasPrefix(id, "m_") {
			if err := tx.Set(s.user(userID).Collection("tombstones").Doc(id),
				map[string]any{"deletedAt": time.Now().UTC()}); err != nil {
				return err
			}
		}
		deleted = true
		return nil
	})
	if err != nil {
		return false, wrapTxErr("delete transaction", err)
	}
	return deleted, nil
}

// UpdateGoal edits a goal with the same version / requestId rules.
func (s *Store) UpdateGoal(ctx context.Context, userID int64, id string, p store.GoalPatch) (store.Goal, error) {
	ref := s.goals(userID).Doc(id)
	var result store.Goal
	err := s.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if notFound(err) {
			return store.ErrNotFound
		}
		if err != nil {
			return err
		}
		var cur store.Goal
		if err := snap.DataTo(&cur); err != nil {
			return err
		}
		cur.ID = id
		cur.Version = version(cur.Version)
		if p.RequestID != "" && cur.LastRequestID == p.RequestID {
			result = cur
			return nil
		}
		if p.Version != cur.Version {
			return &store.ConflictError{Current: cur}
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
		now := time.Now().UTC()
		nw.Version, nw.UpdatedAt, nw.LastRequestID = cur.Version+1, &now, p.RequestID
		if err := tx.Set(ref, nw); err != nil {
			return err
		}
		result = nw
		return nil
	})
	if err != nil {
		return store.Goal{}, wrapTxErr("update goal", err)
	}
	return result, nil
}

// DeleteGoal deletes a goal; a missing goal is not an error.
func (s *Store) DeleteGoal(ctx context.Context, userID int64, id string, ver int64) error {
	ref := s.goals(userID).Doc(id)
	err := s.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		var cur store.Goal
		if err := snap.DataTo(&cur); err != nil {
			return err
		}
		cur.ID = id
		cur.Version = version(cur.Version)
		if ver != cur.Version {
			return &store.ConflictError{Current: cur}
		}
		return tx.Delete(ref)
	})
	if err != nil {
		return wrapTxErr("delete goal", err)
	}
	return nil
}

// wrapTxErr passes typed store errors through unchanged.
func wrapTxErr(op string, err error) error {
	var le *store.LimitError
	var ce *store.ConflictError
	if errors.As(err, &le) || errors.As(err, &ce) || errors.Is(err, store.ErrNotFound) {
		return err
	}
	return fmt.Errorf("%s: %w", op, err)
}
