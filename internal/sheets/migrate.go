package sheets

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

// RepaymentCategory is the main-sheet mirror row of a credit repayment. The _credit sheet
// row already carries the repayment, so these rows are skipped (research R4, R10).
const RepaymentCategory = "Погашение кредита"

// ErrSkip marks rows that carry no ledger entry (zero amount, no month, repayment mirror).
var ErrSkip = errors.New("skip row")

// MigrationID is a deterministic document ID so re-running the migration overwrites.
func MigrationID(sheet string, index int) string {
	sum := sha1.Sum([]byte(sheet + "|" + strconv.Itoa(index)))
	return "m_" + hex.EncodeToString(sum[:])[:20]
}

// SignedAmount parses a row's amount into signed diram. Rows the legacy code would skip
// (empty month, unparsable amount) return ErrSkip or an error.
func SignedAmount(r Row) (int64, error) {
	if strings.TrimSpace(r.Month) == "" {
		return 0, ErrSkip
	}
	if _, err := ledger.ParseMonth(r.Month); err != nil {
		return 0, fmt.Errorf("row %d: %w", r.Index, err)
	}
	a, err := ledger.ParseSomoni(r.Amount)
	if err != nil {
		return 0, fmt.Errorf("row %d: %w", r.Index, err)
	}
	if a == 0 {
		return 0, ErrSkip
	}
	return a, nil
}

func baseTx(sheet string, r Row, kind ledger.Kind, cat string, amount int64) store.Transaction {
	return store.Transaction{
		ID:        MigrationID(sheet, r.Index),
		Kind:      kind,
		Category:  cat,
		Amount:    amount,
		Note:      strings.TrimSpace(r.Note),
		Month:     r.Month,
		Source:    "migration",
		CreatedAt: parseCreatedAt(r.Date, r.Time, r.Month),
	}
}

// MapMain maps a <id> sheet row: positive → income, negative → expense,
// the repayment mirror category → ErrSkip.
func MapMain(sheet string, r Row) (store.Transaction, error) {
	a, err := SignedAmount(r)
	if err != nil {
		return store.Transaction{}, err
	}
	cat := strings.TrimSpace(r.Category)
	if cat == RepaymentCategory {
		return store.Transaction{}, ErrSkip
	}
	if a > 0 {
		return baseTx(sheet, r, ledger.KindIncome, "", a), nil
	}
	if cat == "" {
		cat = "Без категории"
	}
	return baseTx(sheet, r, ledger.KindExpense, cat, -a), nil
}

// MapSavings maps a <id>_savings row: positive → deposit, negative → withdrawal.
func MapSavings(sheet string, r Row) (store.Transaction, error) {
	a, err := SignedAmount(r)
	if err != nil {
		return store.Transaction{}, err
	}
	if a > 0 {
		return baseTx(sheet, r, ledger.KindSavingsDeposit, "", a), nil
	}
	return baseTx(sheet, r, ledger.KindSavingsWithdrawal, "", -a), nil
}

// MapCredit maps a <id>_credit row: positive → purchase (with category), negative → repayment.
func MapCredit(sheet string, r Row) (store.Transaction, error) {
	a, err := SignedAmount(r)
	if err != nil {
		return store.Transaction{}, err
	}
	if a > 0 {
		cat := strings.TrimSpace(r.Category)
		if cat == "" {
			cat = "Без категории"
		}
		return baseTx(sheet, r, ledger.KindCreditPurchase, cat, a), nil
	}
	return baseTx(sheet, r, ledger.KindCreditRepayment, "", -a), nil
}

// MapGoal maps a <id>_goals row.
func MapGoal(sheet string, r GoalRow) (store.Goal, error) {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return store.Goal{}, ErrSkip
	}
	target, err := ledger.ParseSomoni(r.Target)
	if err != nil {
		return store.Goal{}, fmt.Errorf("goal row %d: %w", r.Index, err)
	}
	if target < 0 {
		target = -target
	}
	status := "active"
	if strings.TrimSpace(r.Status) == "Выполнена" {
		status = "done"
	}
	return store.Goal{
		ID:        MigrationID(sheet, r.Index),
		Name:      name,
		Target:    target,
		Quarter:   strings.TrimSpace(r.Quarter),
		Status:    status,
		Note:      strings.TrimSpace(r.Note),
		CreatedAt: time.Date(2000, 1, 1, 0, 0, 0, r.Index, time.UTC), // keeps sheet order
	}, nil
}

// MapUser maps a registry row; registeredAt falls back to the zero time when unparsable.
func MapUser(u UserRow) store.User {
	reg, _ := time.Parse("02.01.2006 15:04", strings.TrimSpace(u.RegisteredAt))
	return store.User{
		ID:           u.ID,
		FirstName:    strings.TrimSpace(u.FirstName),
		Username:     strings.TrimPrefix(strings.TrimSpace(u.Username), "@"),
		RegisteredAt: reg,
	}
}

// parseCreatedAt reads "02.01.2006" + "15:04" as UTC (the old Render server clock);
// it falls back to the first day of the row's month.
func parseCreatedAt(date, tm, month string) time.Time {
	date, tm = strings.TrimSpace(date), strings.TrimSpace(tm)
	for _, layout := range []string{"02.01.2006 15:04", "02.01.2006 15:04:05", "2.1.2006 15:04"} {
		if t, err := time.Parse(layout, date+" "+tm); err == nil {
			return t
		}
	}
	if t, err := time.Parse("02.01.2006", date); err == nil {
		return t
	}
	t, _ := time.Parse("2006-01", month)
	return t
}
