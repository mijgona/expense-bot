package ledger

import (
	"reflect"
	"testing"
	"time"
)

func TestApplyEachKind(t *testing.T) {
	tests := []struct {
		kind    Kind
		want    Month
		savings int64
		debt    int64
	}{
		{KindIncome, Month{Income: 500, CashNet: 500}, 0, 0},
		{KindExpense, Month{Expense: 500, ByCategory: map[string]int64{"Еда": 500}, CashNet: -500}, 0, 0},
		{KindSavingsDeposit, Month{SavingsNet: 500, CashNet: -500}, 500, 0},
		{KindSavingsWithdrawal, Month{SavingsNet: -500, CashNet: 500}, -500, 0},
		{KindCreditPurchase, Month{CreditCharged: 500, ByCreditCategory: map[string]int64{"Еда": 500}}, 0, 500},
		{KindCreditRepayment, Month{CreditRepaid: 500, CashNet: -500}, 0, -500},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			cat := ""
			if tt.kind.NeedsCategory() {
				cat = "Еда"
			}
			d := Apply(tt.kind, cat, 500)
			if !reflect.DeepEqual(d.Month, tt.want) {
				t.Errorf("month = %+v, want %+v", d.Month, tt.want)
			}
			if d.SavingsBalance != tt.savings || d.CreditDebt != tt.debt {
				t.Errorf("savings/debt = %d/%d, want %d/%d", d.SavingsBalance, d.CreditDebt, tt.savings, tt.debt)
			}
		})
	}
}

func TestMixedSequence(t *testing.T) {
	ops := []struct {
		k   Kind
		cat string
		a   int64
	}{
		{KindIncome, "", 10000},
		{KindExpense, "Еда", 3000},
		{KindSavingsDeposit, "", 1000},
		{KindSavingsWithdrawal, "", 300},
		{KindCreditPurchase, "Транспорт", 2000},
		{KindCreditRepayment, "", 500},
	}
	var m Month
	var savings, debt int64
	for _, op := range ops {
		d := Apply(op.k, op.cat, op.a)
		m.Add(d.Month)
		savings += d.SavingsBalance
		debt += d.CreditDebt
	}
	if m.CashNet != 5800 {
		t.Errorf("CashNet = %d, want 5800", m.CashNet)
	}
	if got := m.Income - m.Expense - m.SavingsNet - m.CreditRepaid; got != m.CashNet {
		t.Errorf("identity broken: %d != %d", got, m.CashNet)
	}
	if savings != 700 || debt != 1500 {
		t.Errorf("savings/debt = %d/%d, want 700/1500", savings, debt)
	}
}

func TestParseKind(t *testing.T) {
	if _, err := ParseKind("expense"); err != nil {
		t.Error(err)
	}
	if _, err := ParseKind("refund"); err == nil {
		t.Error("expected error for unknown kind")
	}
}

func TestCarryOver(t *testing.T) {
	got := CarryOver([]Month{{CashNet: 1000}, {CashNet: -300}, {CashNet: 50}})
	if got != 750 {
		t.Errorf("CarryOver = %d, want 750", got)
	}
}

func cats() []CategoryInfo {
	return []CategoryInfo{
		{ID: "c_food", Label: "🍽 Еда", Limit: 200000, Position: 0},
		{ID: "c_transport", Label: "🚗 Транспорт", Limit: 100000, Position: 1},
		{ID: "c_phone", Label: "📱 Связь", Limit: 20000, Position: 2},
		{ID: "c_clothes", Label: "👗 Одежда", Limit: 100000, Position: 3},
		{ID: "c_kids", Label: "🎒 Дети", Limit: 300000, Position: 4, Hidden: true},
		{ID: "c_old", Label: "Старое", Limit: 0, Position: 5, Hidden: true},
	}
}

func TestBuildCategoryLines(t *testing.T) {
	m := Month{ByCategory: map[string]int64{
		"c_food":      160000, // 80% exactly → ok
		"c_phone":     20001,  // >100% → over
		"c_transport": 80001,  // >80% → warn
		"c_kids":      5000,   // hidden with spending → shown, status none
		"c_ghost":     700,    // unknown key → shown raw
	}}
	lines := BuildCategoryLines(m, cats())
	if lines[0].ID != "c_food" || lines[1].ID != "c_transport" || lines[2].ID != "c_phone" {
		t.Fatalf("order = %s, %s, %s", lines[0].ID, lines[1].ID, lines[2].ID)
	}
	got := map[string]CategoryLine{}
	for _, l := range lines {
		got[l.ID] = l
	}
	want := map[string]string{"c_food": "ok", "c_transport": "warn", "c_phone": "over", "c_clothes": "ok", "c_kids": "none", "c_ghost": "none"}
	for k, v := range want {
		if got[k].Status != v {
			t.Errorf("status[%s] = %q, want %q", k, got[k].Status, v)
		}
	}
	if _, ok := got["c_old"]; ok {
		t.Error("hidden category without spending is listed")
	}
	if !got["c_kids"].Hidden || got["c_ghost"].Limit != nil || got["c_ghost"].Label != "c_ghost" {
		t.Errorf("hidden/unknown lines: %+v %+v", got["c_kids"], got["c_ghost"])
	}
}

func TestParseSomoni(t *testing.T) {
	ok := map[string]int64{
		"350":     35000,
		"350.5":   35050,
		"350,50":  35050,
		"1 234,5": 123450,
		"-350":    -35000,
		"−12,5":   -1250,
		"0.005":   1,
		"0.004":   0,
		"10":      1000,
	}
	for in, want := range ok {
		got, err := ParseSomoni(in)
		if err != nil || got != want {
			t.Errorf("ParseSomoni(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"abc", "", "1.2.3", "12,", ",5", "1e3"} {
		if _, err := ParseSomoni(in); err == nil {
			t.Errorf("ParseSomoni(%q) expected error", in)
		}
	}
}

func TestValidate(t *testing.T) {
	if ValidateAmount(0) == nil || ValidateAmount(MaxAmount+1) == nil || ValidateAmount(1) != nil {
		t.Error("ValidateAmount bounds wrong")
	}
	long := make([]rune, 201)
	for i := range long {
		long[i] = 'я'
	}
	if _, err := ValidateNote(string(long)); err == nil {
		t.Error("expected note too long")
	}
	if s, _ := ValidateNote("  такси "); s != "такси" {
		t.Errorf("note not trimmed: %q", s)
	}
}

func TestMonthKeyDushanbe(t *testing.T) {
	utc := time.Date(2026, 10, 31, 19, 30, 0, 0, time.UTC) // 00:30 Nov 1 in Dushanbe (UTC+5)
	if got := MonthKey(utc); got != "2026-11" {
		t.Errorf("MonthKey = %s, want 2026-11", got)
	}
	nowFunc = func() time.Time { return utc }
	defer func() { nowFunc = time.Now }()
	if Now().Day() != 1 {
		t.Errorf("Now() not in Dushanbe: %v", Now())
	}
}

func TestMonthHelpers(t *testing.T) {
	if PrevMonth("2026-01") != "2025-12" || NextMonth("2026-12") != "2027-01" {
		t.Error("prev/next month wrong")
	}
	if _, err := ParseMonth("2026-13"); err == nil {
		t.Error("expected invalid month")
	}
	if d := DaysInMonth(time.Date(2028, 2, 10, 12, 0, 0, 0, Location)); d != 29 {
		t.Errorf("DaysInMonth leap = %d", d)
	}
}

func TestAllowedQuarters(t *testing.T) {
	got := AllowedQuarters(time.Date(2026, 12, 15, 12, 0, 0, 0, Location))
	want := []string{"Q4 2026", "Q1 2027", "Q2 2027", "Q3 2027"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AllowedQuarters = %v, want %v", got, want)
	}
	if q := Quarter(time.Date(2026, 4, 1, 0, 0, 0, 0, Location)); q != "Q2 2026" {
		t.Errorf("Quarter = %s", q)
	}
}

func TestDailyBudget(t *testing.T) {
	days, per := DailyBudget(3000, time.Date(2026, 10, 30, 12, 0, 0, 0, Location))
	if days != 2 || per != 1500 {
		t.Errorf("DailyBudget = %d, %d; want 2, 1500", days, per)
	}
}

func TestFormatSomoni(t *testing.T) {
	cases := map[int64]string{123400: "1 234 с.", 123450: "1 234,50 с.", -500: "−5 с.", 0: "0 с."}
	for in, want := range cases {
		if got := FormatSomoni(in); got != want {
			t.Errorf("FormatSomoni(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRevertNegatesApply(t *testing.T) {
	for _, k := range []Kind{KindExpense, KindIncome, KindSavingsDeposit, KindSavingsWithdrawal, KindCreditPurchase, KindCreditRepayment} {
		var sum Delta
		sum.Add(Apply(k, "Еда", 700))
		sum.Add(Revert(k, "Еда", 700))
		m := sum.Month
		if m.Income|m.Expense|m.SavingsNet|m.CreditCharged|m.CreditRepaid|m.CashNet|sum.SavingsBalance|sum.CreditDebt != 0 {
			t.Errorf("%s: Apply+Revert = %+v", k, sum)
		}
		for c, v := range m.ByCategory {
			if v != 0 {
				t.Errorf("%s: byCategory[%s]=%d", k, c, v)
			}
		}
		for c, v := range m.ByCreditCategory {
			if v != 0 {
				t.Errorf("%s: byCreditCategory[%s]=%d", k, c, v)
			}
		}
	}
}

func TestEditDelta(t *testing.T) {
	var d Delta
	d.Add(Revert(KindExpense, "Транспорт", 35000))
	d.Add(Apply(KindExpense, "Еда", 53000))
	m := d.Month
	if m.Expense != 18000 || m.CashNet != -18000 || m.ByCategory["Транспорт"] != -35000 || m.ByCategory["Еда"] != 53000 {
		t.Errorf("edit delta = %+v", m)
	}
}

func TestBuildCategoryLinesLimitZeroAndPositionTies(t *testing.T) {
	c := cats()
	c[2].Limit = 0 // Связь: no limit
	lines := BuildCategoryLines(Month{ByCategory: map[string]int64{"c_phone": 50000}}, c)
	if lines[0].ID != "c_phone" || lines[0].Status != "none" || *lines[0].Limit != 0 {
		t.Errorf("limit 0 line = %+v", lines[0])
	}
	// equal (zero) spending: ordered by position
	if lines[1].ID != "c_food" || lines[2].ID != "c_transport" {
		t.Errorf("ties = %s, %s", lines[1].ID, lines[2].ID)
	}
}
