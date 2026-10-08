package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"expense-bot/internal/store"
)

func (s *Store) payouts(userID int64) *firestore.CollectionRef {
	return s.user(userID).Collection("payouts")
}

// GetPayouts returns the month's payout states keyed by kind.
func (s *Store) GetPayouts(ctx context.Context, userID int64, month string) (map[string]store.Payout, error) {
	out := map[string]store.Payout{}
	it := s.payouts(userID).Where("month", "==", month).Documents(ctx)
	defer it.Stop()
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get payouts: %w", err)
		}
		var p store.Payout
		if err := snap.DataTo(&p); err != nil {
			return nil, err
		}
		out[p.Kind] = p
	}
}

// DismissPayout hides a payment's offer for the month (idempotent).
func (s *Store) DismissPayout(ctx context.Context, userID int64, month, kind string) error {
	_, err := s.payouts(userID).Doc(month+"_"+kind).Set(ctx, map[string]any{
		"month": month, "kind": kind, "dismissedAt": time.Now().UTC(),
	}, firestore.MergeAll)
	if err != nil {
		return fmt.Errorf("dismiss payout: %w", err)
	}
	return nil
}

// ClaimReminder marks the reminder as sent inside a transaction (at most one per payment).
func (s *Store) ClaimReminder(ctx context.Context, userID int64, month, kind, txID string) (bool, error) {
	ref := s.payouts(userID).Doc(month + "_" + kind)
	txRef := s.txs(userID).Doc(txID)
	claimed := false
	err := s.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		claimed = false
		snap, err := tx.Get(ref)
		if err != nil && !notFound(err) {
			return err
		}
		if snap != nil && snap.Exists() {
			var p store.Payout
			if err := snap.DataTo(&p); err != nil {
				return err
			}
			if p.ReminderSentAt != nil || p.DismissedAt != nil {
				return nil
			}
		}
		rec, err := tx.Get(txRef)
		if err != nil && !notFound(err) {
			return err
		}
		if rec != nil && rec.Exists() {
			return nil // already recorded
		}
		claimed = true
		return tx.Set(ref, map[string]any{"month": month, "kind": kind, "reminderSentAt": time.Now().UTC()}, firestore.MergeAll)
	})
	if err != nil {
		return false, fmt.Errorf("claim reminder: %w", err)
	}
	return claimed, nil
}
