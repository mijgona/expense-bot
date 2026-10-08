package firestore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

const maxScan = 500 // documents scanned per call when searching by note (research R5)

type cursor struct {
	O time.Time `json:"o"`
	C time.Time `json:"c"`
}

func encodeCursor(t store.Transaction) string {
	b, _ := json.Marshal(cursor{O: t.When(), C: t.CreatedAt})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (cursor, error) {
	var c cursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	if err != nil {
		return c, fmt.Errorf("invalid cursor")
	}
	return c, nil
}

// filtered applies the History filters (without order / cursor).
func (s *Store) filtered(userID int64, f store.HistoryFilter) (firestore.Query, error) {
	q := s.txs(userID).Query
	switch len(f.Kinds) {
	case 0:
	case 1:
		q = q.Where("kind", "==", string(f.Kinds[0]))
	default:
		kinds := make([]string, len(f.Kinds))
		for i, k := range f.Kinds {
			kinds[i] = string(k)
		}
		q = q.Where("kind", "in", kinds)
	}
	if f.Category != "" {
		q = q.Where("category", "==", f.Category)
	}
	if f.Month != "" {
		start, err := ledger.ParseMonth(f.Month)
		if err != nil {
			return q, err
		}
		q = q.Where("occurredAt", ">=", start).Where("occurredAt", "<", start.AddDate(0, 1, 0))
	}
	return q, nil
}

// QueryTransactions returns one History page, newest first.
func (s *Store) QueryTransactions(ctx context.Context, userID int64, f store.HistoryFilter) ([]store.Transaction, string, error) {
	base, err := s.filtered(userID, f)
	if err != nil {
		return nil, "", err
	}
	base = base.OrderBy("occurredAt", firestore.Desc).OrderBy("createdAt", firestore.Desc)
	if f.Cursor != "" {
		c, err := decodeCursor(f.Cursor)
		if err != nil {
			return nil, "", err
		}
		base = base.StartAfter(c.O, c.C)
	}
	needle := strings.ToLower(strings.TrimSpace(f.Q))

	if needle == "" {
		items, err := readTx(ctx, base.Limit(f.Limit+1))
		if err != nil {
			return nil, "", err
		}
		next := ""
		if len(items) > f.Limit {
			items = items[:f.Limit]
			next = encodeCursor(items[len(items)-1])
		}
		return items, next, nil
	}

	// Note search: scan pages and filter in memory, up to maxScan documents.
	var out []store.Transaction
	q, scanned, page := base, 0, 100
	for {
		batch, err := readTx(ctx, q.Limit(page))
		if err != nil {
			return nil, "", err
		}
		for _, t := range batch {
			scanned++
			if strings.Contains(strings.ToLower(t.Note), needle) {
				out = append(out, t)
			}
			if len(out) == f.Limit || scanned >= maxScan {
				return out, encodeCursor(t), nil
			}
		}
		if len(batch) < page {
			return out, "", nil
		}
		last := batch[len(batch)-1]
		q = base.StartAfter(last.When(), last.CreatedAt)
	}
}

func readTx(ctx context.Context, q firestore.Query) ([]store.Transaction, error) {
	it := q.Documents(ctx)
	defer it.Stop()
	var out []store.Transaction
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("query transactions: %w", err)
		}
		var t store.Transaction
		if err := snap.DataTo(&t); err != nil {
			return nil, err
		}
		t.ID = snap.Ref.ID
		out = append(out, t)
	}
}

// SumTransactions returns the sum of amounts for the filters (server-side aggregation).
func (s *Store) SumTransactions(ctx context.Context, userID int64, f store.HistoryFilter) (int64, error) {
	q, err := s.filtered(userID, f)
	if err != nil {
		return 0, err
	}
	res, err := q.NewAggregationQuery().WithSum("amount", "s").Get(ctx)
	if err != nil {
		return 0, fmt.Errorf("sum transactions: %w", err)
	}
	switch v := res["s"].(type) {
	case interface{ GetIntegerValue() int64 }:
		if n := v.GetIntegerValue(); n != 0 {
			return n, nil
		}
		if d, ok := v.(interface{ GetDoubleValue() float64 }); ok {
			return int64(d.GetDoubleValue()), nil
		}
	}
	return 0, nil
}

// ListMonths returns stored month aggregates in [from, to], newest first.
func (s *Store) ListMonths(ctx context.Context, userID int64, from, to string) ([]ledger.Month, error) {
	q := s.months(userID).Query
	if from != "" {
		q = q.Where("month", ">=", from)
	}
	if to != "" {
		q = q.Where("month", "<=", to)
	}
	ms, err := s.queryMonths(ctx, q.OrderBy("month", firestore.Desc))
	if err != nil {
		return nil, err
	}
	// Months whose records were all moved or deleted keep a zero doc; hide them.
	out := ms[:0]
	for _, m := range ms {
		if m.Income != 0 || m.Expense != 0 || m.SavingsNet != 0 || m.CreditCharged != 0 || m.CreditRepaid != 0 {
			out = append(out, m)
		}
	}
	return out, nil
}
