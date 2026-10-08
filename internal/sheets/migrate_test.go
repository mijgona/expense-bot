package sheets

import (
	"errors"
	"testing"
	"time"

	"expense-bot/internal/ledger"
)

func row(cat, amount, month string) Row {
	return Row{Index: 5, Date: "15.03.2026", Time: "09:30", Category: cat, Amount: amount, Note: " такси ", Month: month}
}

func TestMapMain(t *testing.T) {
	tx, err := MapMain("42", row("Транспорт", "-350", "2026-03"))
	if err != nil {
		t.Fatal(err)
	}
	if tx.Kind != ledger.KindExpense || tx.Amount != 35000 || tx.Category != "Транспорт" || tx.Note != "такси" || tx.Month != "2026-03" {
		t.Errorf("expense = %+v", tx)
	}
	if want := time.Date(2026, 3, 15, 9, 30, 0, 0, time.UTC); !tx.CreatedAt.Equal(want) {
		t.Errorf("createdAt = %v, want %v", tx.CreatedAt, want)
	}
	if tx.Source != "migration" {
		t.Errorf("source = %q", tx.Source)
	}

	tx, err = MapMain("42", row("Приход", "1 234,50", "2026-03"))
	if err != nil || tx.Kind != ledger.KindIncome || tx.Amount != 123450 || tx.Category != "" {
		t.Errorf("income = %+v, %v", tx, err)
	}

	tx, err = MapMain("42", row("Старая категория", "-10", "2025-01"))
	if err != nil || tx.Category != "Старая категория" {
		t.Errorf("legacy category = %+v, %v", tx, err)
	}

	if _, err := MapMain("42", row(RepaymentCategory, "-500", "2026-03")); !errors.Is(err, ErrSkip) {
		t.Errorf("repayment mirror: err = %v, want ErrSkip", err)
	}
	if _, err := MapMain("42", row("Еда", "0", "2026-03")); !errors.Is(err, ErrSkip) {
		t.Errorf("zero: err = %v, want ErrSkip", err)
	}
	if _, err := MapMain("42", row("Еда", "-5", "")); !errors.Is(err, ErrSkip) {
		t.Errorf("no month: err = %v, want ErrSkip", err)
	}
	if _, err := MapMain("42", row("Еда", "abc", "2026-03")); err == nil || errors.Is(err, ErrSkip) {
		t.Errorf("bad amount: err = %v, want parse error", err)
	}
}

func TestMapSavings(t *testing.T) {
	tx, err := MapSavings("42_savings", row("", "1000", "2026-03"))
	if err != nil || tx.Kind != ledger.KindSavingsDeposit || tx.Amount != 100000 {
		t.Errorf("deposit = %+v, %v", tx, err)
	}
	tx, err = MapSavings("42_savings", row("", "-300", "2026-03"))
	if err != nil || tx.Kind != ledger.KindSavingsWithdrawal || tx.Amount != 30000 {
		t.Errorf("withdrawal = %+v, %v", tx, err)
	}
}

func TestMapCredit(t *testing.T) {
	tx, err := MapCredit("42_credit", row("Еда", "2000", "2026-03"))
	if err != nil || tx.Kind != ledger.KindCreditPurchase || tx.Category != "Еда" || tx.Amount != 200000 {
		t.Errorf("purchase = %+v, %v", tx, err)
	}
	tx, err = MapCredit("42_credit", row("Погашение", "-500", "2026-03"))
	if err != nil || tx.Kind != ledger.KindCreditRepayment || tx.Category != "" || tx.Amount != 50000 {
		t.Errorf("repayment = %+v, %v", tx, err)
	}
}

func TestMapGoalAndUser(t *testing.T) {
	g, err := MapGoal("42_goals", GoalRow{Index: 2, Name: "Отпуск", Target: "3000", Quarter: "Q4 2026", Status: "Выполнена"})
	if err != nil || g.Status != "done" || g.Target != 300000 {
		t.Errorf("goal = %+v, %v", g, err)
	}
	g, _ = MapGoal("42_goals", GoalRow{Index: 3, Name: "Машина", Target: "50000", Status: ""})
	if g.Status != "active" {
		t.Errorf("status = %q, want active", g.Status)
	}
	u := MapUser(UserRow{ID: 42, FirstName: "Алиса", Username: "@alice", RegisteredAt: "10.06.2026 12:00"})
	if u.Username != "alice" || u.RegisteredAt.IsZero() {
		t.Errorf("user = %+v", u)
	}
}

func TestMigrationIDDeterministic(t *testing.T) {
	a, b := MigrationID("42", 7), MigrationID("42", 7)
	if a != b || len(a) != 22 {
		t.Errorf("ids %q %q", a, b)
	}
	if MigrationID("42", 7) == MigrationID("42_savings", 7) {
		t.Error("ids collide across sheets")
	}
}
