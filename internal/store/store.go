// Package store defines persistence for users, the ledger, month aggregates and goals.
// The production implementation is store/firestore.
package store

import (
	"context"
	"errors"
	"time"

	"expense-bot/internal/catalog"
	"expense-bot/internal/ledger"
)

var (
	ErrInsufficientSavings = errors.New("insufficient savings")
	ErrExceedsDebt         = errors.New("repayment exceeds debt")
	ErrNotFound            = errors.New("not found")
)

// LimitError reports a change that would make savings or card debt newly or more negative
// at some date (constitution II, ledger.CheckRunning). errors.Is matches
// ErrInsufficientSavings / ErrExceedsDebt.
type LimitError struct {
	Err       error
	Available int64     // current balance (informational)
	At        time.Time // first date where the balance would break
	Balance   int64     // the would-be balance at At (negative)
}

// CategoryState is the user's category list with its version (ConflictError.Current for lists).
type CategoryState struct {
	List    catalog.List
	Version int64
}

// ConflictError reports a stale version; Current holds the fresh Transaction, Goal or CategoryState.
type ConflictError struct {
	Current any
}

func (e *ConflictError) Error() string { return "conflict: changed elsewhere" }

func (e *LimitError) Error() string { return e.Err.Error() }
func (e *LimitError) Unwrap() error { return e.Err }

// User is a registered Telegram user with running balances (diram) and profile overrides.
type User struct {
	ID             int64     `firestore:"-"`
	FirstName      string    `firestore:"firstName"`
	Username       string    `firestore:"username"`
	RegisteredAt   time.Time `firestore:"registeredAt"`
	LastSeenAt     time.Time `firestore:"lastSeenAt"`
	SavingsBalance int64     `firestore:"savingsBalance"`
	CreditDebt     int64     `firestore:"creditDebt"`

	DisplayName      *string    `firestore:"displayName,omitempty"`
	Salary           *int64     `firestore:"salary,omitempty"`
	ProfileUpdatedAt *time.Time `firestore:"profileUpdatedAt,omitempty"`

	// Categories is the user's own category list (feature 006); empty before the conversion.
	Categories        catalog.List `firestore:"categories,omitempty"`
	CategoriesVersion int64        `firestore:"categoriesVersion"`
}

// Overrides returns the user's profile overrides for ledger.NewEffective.
func (u User) Overrides() ledger.ProfileOverrides {
	return ledger.ProfileOverrides{DisplayName: u.DisplayName, Salary: u.Salary}
}

// CategoryList returns the user's categories, or the defaults when none are stored yet.
func (u User) CategoryList() catalog.List {
	if len(u.Categories) == 0 {
		return catalog.Defaults(u.RegisteredAt)
	}
	return u.Categories
}

// Transaction is one ledger entry. Amount is always positive (diram); Kind never changes.
// OccurredAt is the business date (month derives from it); CreatedAt is when it was entered.
type Transaction struct {
	ID            string      `firestore:"-"`
	Kind          ledger.Kind `firestore:"kind"`
	Category      string      `firestore:"category"`
	Amount        int64       `firestore:"amount"`
	Note          string      `firestore:"note"`
	Month         string      `firestore:"month"`
	Source        string      `firestore:"source"`
	OccurredAt    time.Time   `firestore:"occurredAt"`
	CreatedAt     time.Time   `firestore:"createdAt"`
	Version       int64       `firestore:"version"`
	EditedAt      *time.Time  `firestore:"editedAt,omitempty"`
	LastRequestID string      `firestore:"lastRequestId,omitempty"`
}

// When returns OccurredAt, falling back to CreatedAt for records not yet backfilled.
func (t Transaction) When() time.Time {
	if t.OccurredAt.IsZero() {
		return t.CreatedAt
	}
	return t.OccurredAt
}

// TransactionPatch is a partial edit; nil fields are unchanged. Date replaces the day only
// (the time of day is kept).
type TransactionPatch struct {
	Version   int64
	RequestID string
	Amount    *int64
	Category  *string
	Note      *string
	Date      *time.Time
}

// HistoryFilter selects records for the History screen.
type HistoryFilter struct {
	Kinds    []ledger.Kind
	Category string
	Month    string
	Q        string
	Cursor   string
	Limit    int
}

// Goal is a quarterly savings goal.
type Goal struct {
	ID            string     `firestore:"-"`
	Name          string     `firestore:"name"`
	Target        int64      `firestore:"target"`
	Quarter       string     `firestore:"quarter"`
	Status        string     `firestore:"status"` // active | done
	Note          string     `firestore:"note"`
	CreatedAt     time.Time  `firestore:"createdAt"`
	Version       int64      `firestore:"version"`
	UpdatedAt     *time.Time `firestore:"updatedAt,omitempty"`
	LastRequestID string     `firestore:"lastRequestId,omitempty"`
}

// GoalPatch is a partial goal edit; nil fields are unchanged.
type GoalPatch struct {
	Version   int64
	RequestID string
	Name      *string
	Target    *int64
	Quarter   *string
	Note      *string
	Status    *string
}

// ProfilePatch changes profile overrides. For each field: Set=false → unchanged;
// Set=true with nil Value → reset to the shared default.
type ProfilePatch struct {
	DisplayName OptString
	Salary      OptInt
}

type OptString struct {
	Set   bool
	Value *string
}

type OptInt struct {
	Set   bool
	Value *int64
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

	// GetTransaction returns one record or ErrNotFound.
	GetTransaction(ctx context.Context, userID int64, id string) (Transaction, error)
	// UpdateTransaction edits in place via Revert+Apply in one transaction. Idempotent by
	// RequestID; stale Version → *ConflictError; balance rule → *LimitError; missing → ErrNotFound.
	UpdateTransaction(ctx context.Context, userID int64, id string, p TransactionPatch) (Transaction, error)
	// DeleteTransaction reverts and deletes; deleted=false when it did not exist.
	DeleteTransaction(ctx context.Context, userID int64, id string, version int64) (deleted bool, err error)
	// QueryTransactions returns a History page (occurredAt desc, createdAt desc) and the next cursor.
	QueryTransactions(ctx context.Context, userID int64, f HistoryFilter) (items []Transaction, next string, err error)
	// SumTransactions sums amounts matching f (Q and Cursor ignored).
	SumTransactions(ctx context.Context, userID int64, f HistoryFilter) (int64, error)
	// ListMonths returns stored month aggregates in [from, to] (empty = open), newest first.
	ListMonths(ctx context.Context, userID int64, from, to string) ([]ledger.Month, error)

	ListGoals(ctx context.Context, userID int64) ([]Goal, error)
	// AddGoal is idempotent by g.ID.
	AddGoal(ctx context.Context, userID int64, g Goal) (stored Goal, created bool, err error)
	UpdateGoal(ctx context.Context, userID int64, id string, p GoalPatch) (Goal, error)
	DeleteGoal(ctx context.Context, userID int64, id string, version int64) error

	UpdateProfile(ctx context.Context, userID int64, p ProfilePatch) (User, error)

	// UpdateCategories is a transactional read-modify-write of the user's category list.
	// version 0 skips the version check; a stale version → *ConflictError{Current: list};
	// errors from fn are returned unchanged. Returns the new list and categoriesVersion.
	UpdateCategories(ctx context.Context, userID int64, version int64, fn func(catalog.List) error) (catalog.List, int64, error)

	ListUserIDs(ctx context.Context) ([]int64, error)
	Close() error
}
