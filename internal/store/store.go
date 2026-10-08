// Package store defines persistence for users, the ledger, month aggregates and goals.
// The production implementation is store/firestore.
package store

import (
	"context"
	"errors"
	"time"

	"expense-bot/internal/ledger"
)

var (
	ErrInsufficientSavings = errors.New("insufficient savings")
	ErrExceedsDebt         = errors.New("repayment exceeds debt")
)

// LimitError reports a rejected withdrawal/repayment with the amount that was available.
// errors.Is(err, ErrInsufficientSavings / ErrExceedsDebt) matches it.
type LimitError struct {
	Err       error
	Available int64
}

func (e *LimitError) Error() string { return e.Err.Error() }
func (e *LimitError) Unwrap() error { return e.Err }

// User is a registered Telegram user with running balances (diram).
type User struct {
	ID             int64     `firestore:"-"`
	FirstName      string    `firestore:"firstName"`
	Username       string    `firestore:"username"`
	RegisteredAt   time.Time `firestore:"registeredAt"`
	LastSeenAt     time.Time `firestore:"lastSeenAt"`
	SavingsBalance int64     `firestore:"savingsBalance"`
	CreditDebt     int64     `firestore:"creditDebt"`
}

// Transaction is one immutable ledger entry. Amount is always positive (diram).
type Transaction struct {
	ID        string      `firestore:"-"`
	Kind      ledger.Kind `firestore:"kind"`
	Category  string      `firestore:"category"`
	Amount    int64       `firestore:"amount"`
	Note      string      `firestore:"note"`
	Month     string      `firestore:"month"`
	Source    string      `firestore:"source"`
	CreatedAt time.Time   `firestore:"createdAt"`
}

// Goal is a quarterly savings goal.
type Goal struct {
	ID        string    `firestore:"-"`
	Name      string    `firestore:"name"`
	Target    int64     `firestore:"target"`
	Quarter   string    `firestore:"quarter"`
	Status    string    `firestore:"status"` // active | done
	Note      string    `firestore:"note"`
	CreatedAt time.Time `firestore:"createdAt"`
}

// Store is the persistence contract used by the API, bot and advisor.
// Missing documents read as zero values, never as errors.
type Store interface {
	EnsureUser(ctx context.Context, id int64, firstName, username string) (u User, created bool, err error)
	GetUser(ctx context.Context, id int64) (User, error)

	// AddTransaction is idempotent by t.ID: an existing ID returns the stored entry with created=false.
	// Withdrawals above the savings balance and repayments above the debt return *LimitError.
	AddTransaction(ctx context.Context, userID int64, t Transaction) (stored Transaction, created bool, err error)
	ListTransactions(ctx context.Context, userID int64, month string, limit int) ([]Transaction, error)

	GetMonth(ctx context.Context, userID int64, month string) (ledger.Month, error)
	MonthsBefore(ctx context.Context, userID int64, month string) ([]ledger.Month, error)
	FirstMonth(ctx context.Context, userID int64) (string, error)

	ListGoals(ctx context.Context, userID int64) ([]Goal, error)
	// AddGoal is idempotent by g.ID.
	AddGoal(ctx context.Context, userID int64, g Goal) (stored Goal, created bool, err error)

	ListUserIDs(ctx context.Context) ([]int64, error)
	Close() error
}
