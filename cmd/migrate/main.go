// Command migrate imports the legacy Google Sheets data into Firestore, rebuilds aggregates
// and verifies totals (FR-018, SC-005, SC-007). It is idempotent: re-running overwrites the
// same documents. Not deployed.
//
//	go run ./cmd/migrate -dry-run          # read + map + print, write nothing
//	go run ./cmd/migrate                   # import + rebuild + verify (exit 1 on mismatch)
//	go run ./cmd/migrate -user 42          # one user only
//	go run ./cmd/migrate -rebuild          # rebuild aggregates from Firestore only, then verify
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	"expense-bot/internal/ledger"
	"expense-bot/internal/sheets"
	"expense-bot/internal/store"
	"expense-bot/internal/store/firestore"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "read and map only; write nothing")
	onlyUser := flag.Int64("user", 0, "migrate a single Telegram user ID")
	rebuildOnly := flag.Bool("rebuild", false, "skip import; rebuild aggregates from Firestore transactions and verify")
	backfill005 := flag.Bool("backfill-005", false, "set occurredAt/version on existing records (feature 005), nothing else")
	flag.Parse()

	ctx := context.Background()
	creds := loadCredentials()

	if *backfill005 {
		st := mustStore(ctx, creds)
		defer st.Close()
		ids := []int64{*onlyUser}
		if *onlyUser == 0 {
			var err error
			if ids, err = st.ListUserIDs(ctx); err != nil {
				log.Fatal(err)
			}
		}
		for _, id := range ids {
			txs, goals, err := st.Backfill005(ctx, id)
			if err != nil {
				log.Fatalf("user %d: backfill: %v", id, err)
			}
			fmt.Printf("user %d: backfilled %d transactions, %d goals\n", id, txs, goals)
		}
		return
	}

	if *rebuildOnly {
		st := mustStore(ctx, creds)
		defer st.Close()
		ids := []int64{*onlyUser}
		if *onlyUser == 0 {
			var err error
			if ids, err = st.ListUserIDs(ctx); err != nil {
				log.Fatal(err)
			}
		}
		mismatches := 0
		for _, id := range ids {
			if _, _, err := st.RebuildAggregates(ctx, id); err != nil {
				log.Fatalf("user %d: rebuild: %v", id, err)
			}
			mismatches += verifyAggregates(ctx, st, id)
		}
		finish(len(ids), -1, mismatches)
		return
	}

	spreadsheetID := os.Getenv("SPREADSHEET_ID")
	if spreadsheetID == "" {
		log.Fatal("SPREADSHEET_ID is required")
	}
	sh, err := sheets.NewFromJSON(creds, spreadsheetID)
	if err != nil {
		log.Fatal(err)
	}
	ids, registry, err := sh.UserIDs()
	if err != nil {
		log.Fatal(err)
	}
	if *onlyUser != 0 {
		ids = []int64{*onlyUser}
	}

	var st *firestore.Store
	if !*dryRun {
		st = mustStore(ctx, creds)
		defer st.Close()
	}

	mismatches, monthsChecked := 0, 0
	for _, id := range ids {
		data, err := readUser(sh, id)
		if err != nil {
			log.Fatalf("user %d: %v", id, err)
		}
		data.print(id)
		if *dryRun {
			continue
		}

		u, ok := registry[id]
		if !ok {
			u = sheets.UserRow{ID: id}
		}
		if err := st.UpsertUserRaw(ctx, sheets.MapUser(u)); err != nil {
			log.Fatalf("user %d: upsert user: %v", id, err)
		}
		skippedEdited, skippedDeleted, err := st.UpsertTransactionsRaw(ctx, id, data.txs)
		if err != nil {
			log.Fatalf("user %d: %v", id, err)
		}
		if skippedEdited+skippedDeleted > 0 {
			fmt.Printf("  skipped %d edited, %d deleted-by-user (FR-011)\n", skippedEdited, skippedDeleted)
		}
		if err := st.UpsertGoalsRaw(ctx, id, data.goals); err != nil {
			log.Fatalf("user %d: %v", id, err)
		}
		keepTx, keepGoals := map[string]bool{}, map[string]bool{}
		for _, t := range data.txs {
			keepTx[t.ID] = true
		}
		for _, g := range data.goals {
			keepGoals[g.ID] = true
		}
		if n, err := st.DeleteStaleMigrated(ctx, id, keepTx, keepGoals); err != nil {
			log.Fatalf("user %d: delete stale: %v", id, err)
		} else if n > 0 {
			fmt.Printf("  removed %d stale migrated docs (rows edited/deleted in the sheet)\n", n)
		}
		if _, _, err := st.RebuildAggregates(ctx, id); err != nil {
			log.Fatalf("user %d: rebuild: %v", id, err)
		}
		if skippedEdited+skippedDeleted > 0 {
			// The user changed migrated records in the app, so Firestore intentionally differs
			// from the sheet; only aggregate consistency can be verified.
			fmt.Printf("  sheet comparison skipped (user edits present); checking aggregates only\n")
			mismatches += verifyAggregates(ctx, st, id)
		} else {
			mm, n := verifyAgainstSheets(ctx, st, id, data)
			mismatches += mm + verifyAggregates(ctx, st, id)
			monthsChecked += n
		}
	}
	if *dryRun {
		fmt.Printf("dry-run: %d users read, nothing written\n", len(ids))
		return
	}
	finish(len(ids), monthsChecked, mismatches)
}

// ── reading & expected totals ────────────────────────────────────────────────

// expected holds totals computed straight from sheet rows, independent of ledger.Apply.
type expected struct {
	income, expense, savingsNet, creditCharged, creditRepaid int64
	legacyCash                                               int64 // Σ main signed (incl. repayment mirror) − Σ savings signed
}

type userData struct {
	txs      []store.Transaction
	goals    []store.Goal
	months   map[string]*expected
	savings  int64
	debt     int64
	counts   map[string]map[ledger.Kind]int
	skipped  int
	invalid  int
	mirrorRe int // repayment-mirror rows skipped
}

func (d *userData) month(m string) *expected {
	if d.months[m] == nil {
		d.months[m] = &expected{}
	}
	return d.months[m]
}

func readUser(sh *sheets.Client, id int64) (*userData, error) {
	d := &userData{months: map[string]*expected{}, counts: map[string]map[ledger.Kind]int{}}

	add := func(t store.Transaction) {
		d.txs = append(d.txs, t)
		if d.counts[t.Month] == nil {
			d.counts[t.Month] = map[ledger.Kind]int{}
		}
		d.counts[t.Month][t.Kind]++
	}
	handle := func(sheet string, rows []sheets.Row, mapFn func(string, sheets.Row) (store.Transaction, error),
		expect func(r sheets.Row, signed int64)) {
		for _, r := range rows {
			if signed, err := sheets.SignedAmount(r); err == nil {
				expect(r, signed)
			}
			t, err := mapFn(sheet, r)
			switch {
			case errors.Is(err, sheets.ErrSkip):
				d.skipped++
			case err != nil:
				d.invalid++
				log.Printf("WARN user=%d sheet=%s: %v (row skipped, as the legacy bot did)", id, sheet, err)
			default:
				add(t)
			}
		}
	}

	main, err := sh.ReadMain(sheets.MainSheet(id))
	if err != nil {
		return nil, err
	}
	handle(sheets.MainSheet(id), main, sheets.MapMain, func(r sheets.Row, a int64) {
		e := d.month(r.Month)
		e.legacyCash += a
		switch {
		case r.Category == sheets.RepaymentCategory:
			d.mirrorRe++
		case a > 0:
			e.income += a
		default:
			e.expense += -a
		}
	})

	sav, err := sh.ReadSavings(sheets.SavingsSheet(id))
	if err != nil {
		return nil, err
	}
	handle(sheets.SavingsSheet(id), sav, sheets.MapSavings, func(r sheets.Row, a int64) {
		e := d.month(r.Month)
		e.savingsNet += a
		e.legacyCash -= a
		d.savings += a
	})

	cr, err := sh.ReadMain(sheets.CreditSheet(id))
	if err != nil {
		return nil, err
	}
	handle(sheets.CreditSheet(id), cr, sheets.MapCredit, func(r sheets.Row, a int64) {
		e := d.month(r.Month)
		if a > 0 {
			e.creditCharged += a
		} else {
			e.creditRepaid += -a
		}
		d.debt += a
	})

	goals, err := sh.ReadGoals(sheets.GoalsSheet(id))
	if err != nil {
		return nil, err
	}
	for _, r := range goals {
		g, err := sheets.MapGoal(sheets.GoalsSheet(id), r)
		if errors.Is(err, sheets.ErrSkip) {
			continue
		}
		if err != nil {
			d.invalid++
			log.Printf("WARN user=%d goals: %v", id, err)
			continue
		}
		d.goals = append(d.goals, g)
	}
	return d, nil
}

func (d *userData) print(id int64) {
	fmt.Printf("user %d: %d transactions, %d goals, %d skipped (incl. %d repayment mirrors), %d invalid\n",
		id, len(d.txs), len(d.goals), d.skipped, d.mirrorRe, d.invalid)
	months := make([]string, 0, len(d.counts))
	for m := range d.counts {
		months = append(months, m)
	}
	sort.Strings(months)
	for _, m := range months {
		fmt.Printf("  %s:", m)
		for _, k := range []ledger.Kind{ledger.KindIncome, ledger.KindExpense, ledger.KindSavingsDeposit,
			ledger.KindSavingsWithdrawal, ledger.KindCreditPurchase, ledger.KindCreditRepayment} {
			if n := d.counts[m][k]; n > 0 {
				fmt.Printf(" %s=%d", k, n)
			}
		}
		fmt.Println()
	}
}

// ── verification ─────────────────────────────────────────────────────────────

func mismatch(id int64, month, field string, want, got int64) int {
	if want == got {
		return 0
	}
	fmt.Printf("MISMATCH user=%d month=%s field=%s sheets=%d firestore=%d\n", id, month, field, want, got)
	return 1
}

// verifyAgainstSheets folds the migrated transactions read back from Firestore and compares
// them with totals computed directly from the sheet rows.
func verifyAgainstSheets(ctx context.Context, st *firestore.Store, id int64, d *userData) (int, int) {
	all, err := st.AllTransactions(ctx, id)
	if err != nil {
		log.Fatalf("user %d: read back: %v", id, err)
	}
	var migrated []store.Transaction
	for _, t := range all {
		if t.Source == "migration" {
			migrated = append(migrated, t)
		}
	}
	got, savings, debt := firestore.Fold(migrated)

	n := 0
	for m, e := range d.months {
		g := got[m]
		cash := e.income - e.expense - e.savingsNet - e.creditRepaid
		n += mismatch(id, m, "income", e.income, g.Income)
		n += mismatch(id, m, "expense", e.expense, g.Expense)
		n += mismatch(id, m, "savingsNet", e.savingsNet, g.SavingsNet)
		n += mismatch(id, m, "creditCharged", e.creditCharged, g.CreditCharged)
		n += mismatch(id, m, "creditRepaid", e.creditRepaid, g.CreditRepaid)
		n += mismatch(id, m, "cashNet", cash, g.CashNet)
		// Legacy carry-over formula vs the new one: differs only if repayment mirror rows
		// in the main sheet don't match the _credit sheet.
		n += mismatch(id, m, "cashNet(legacy formula)", e.legacyCash, g.CashNet)
	}
	for m := range got {
		if _, ok := d.months[m]; !ok {
			n += mismatch(id, m, "unexpected month", 0, 1)
		}
	}
	n += mismatch(id, "*", "savingsBalance", d.savings, savings)
	n += mismatch(id, "*", "creditDebt", d.debt, debt)
	return n, len(d.months)
}

// verifyAggregates checks stored month docs and balances against a fold of all transactions.
func verifyAggregates(ctx context.Context, st *firestore.Store, id int64) int {
	all, err := st.AllTransactions(ctx, id)
	if err != nil {
		log.Fatalf("user %d: %v", id, err)
	}
	want, savings, debt := firestore.Fold(all)
	n := 0
	for m, w := range want {
		g, err := st.GetMonth(ctx, id, m)
		if err != nil {
			log.Fatalf("user %d: %v", id, err)
		}
		n += mismatch(id, m, "stored.income", w.Income, g.Income)
		n += mismatch(id, m, "stored.expense", w.Expense, g.Expense)
		n += mismatch(id, m, "stored.savingsNet", w.SavingsNet, g.SavingsNet)
		n += mismatch(id, m, "stored.creditCharged", w.CreditCharged, g.CreditCharged)
		n += mismatch(id, m, "stored.creditRepaid", w.CreditRepaid, g.CreditRepaid)
		n += mismatch(id, m, "stored.cashNet", w.CashNet, g.CashNet)
		for c, v := range w.ByCategory {
			n += mismatch(id, m, "stored.byCategory."+c, v, g.ByCategory[c])
		}
	}
	u, err := st.GetUser(ctx, id)
	if err != nil {
		log.Fatalf("user %d: %v", id, err)
	}
	n += mismatch(id, "*", "stored.savingsBalance", savings, u.SavingsBalance)
	n += mismatch(id, "*", "stored.creditDebt", debt, u.CreditDebt)
	return n
}

func finish(users, months, mismatches int) {
	if months >= 0 {
		fmt.Printf("verify: %d users, %d months, %d mismatches\n", users, months, mismatches)
	} else {
		fmt.Printf("verify: %d users rebuilt, %d mismatches\n", users, mismatches)
	}
	if mismatches > 0 {
		os.Exit(1)
	}
}

// ── setup ────────────────────────────────────────────────────────────────────

func loadCredentials() []byte {
	if raw := os.Getenv("GOOGLE_CREDENTIALS_JSON"); raw != "" {
		return []byte(raw)
	}
	path := os.Getenv("CREDENTIALS_PATH")
	if path == "" {
		path = "credentials.json"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read credentials: %v", err)
	}
	return data
}

func mustStore(ctx context.Context, creds []byte) *firestore.Store {
	st, err := firestore.New(ctx, creds, os.Getenv("FIRESTORE_PROJECT_ID"))
	if err != nil {
		log.Fatal(err)
	}
	return st
}
