package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

// balanceGroup describes a running balance guarded by the chronological rule (research R3).
type balanceGroup struct {
	kinds []string
	delta func(ledger.Kind, int64) (int64, bool)
	err   error
}

func groupFor(k ledger.Kind) *balanceGroup {
	if _, ok := ledger.SavingsDelta(k, 0); ok {
		return &balanceGroup{
			kinds: []string{string(ledger.KindSavingsDeposit), string(ledger.KindSavingsWithdrawal)},
			delta: ledger.SavingsDelta, err: store.ErrInsufficientSavings,
		}
	}
	if _, ok := ledger.DebtDelta(k, 0); ok {
		return &balanceGroup{
			kinds: []string{string(ledger.KindCreditPurchase), string(ledger.KindCreditRepayment)},
			delta: ledger.DebtDelta, err: store.ErrExceedsDebt,
		}
	}
	return nil
}

func event(g *balanceGroup, t store.Transaction) ledger.Event {
	d, _ := g.delta(t.Kind, t.Amount)
	return ledger.Event{At: t.When(), Seq: t.CreatedAt, Delta: d}
}

// checkBalance replays the balance group of kind k inside tx. replaceID (may be "") is removed
// from the "after" set, and next (may be nil) is added. Must be called before any tx writes.
func (s *Store) checkBalance(ctx context.Context, tx *firestore.Transaction, userID int64,
	k ledger.Kind, replaceID string, next *store.Transaction) error {
	g := groupFor(k)
	if g == nil {
		return nil
	}
	it := tx.Documents(s.txs(userID).Where("kind", "in", g.kinds))
	defer it.Stop()
	var before, after []ledger.Event
	var current int64
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		var t store.Transaction
		if err := snap.DataTo(&t); err != nil {
			return err
		}
		e := event(g, t)
		before = append(before, e)
		current += e.Delta
		if snap.Ref.ID != replaceID {
			after = append(after, e)
		}
	}
	if next != nil {
		after = append(after, event(g, *next))
	}
	if v := ledger.CheckRunning(before, after); v != nil {
		return &store.LimitError{Err: g.err, Available: current, At: v.At, Balance: v.Balance}
	}
	return nil
}
