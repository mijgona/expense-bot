package payroll

import (
	"reflect"
	"testing"
	"time"

	"expense-bot/internal/ledger"
)

func d(y, m, day int) time.Time { return time.Date(y, time.Month(m), day, 0, 0, 0, 0, ledger.Location) }

func at(y, m, day, hh int) time.Time {
	return time.Date(y, time.Month(m), day, hh, 0, 0, 0, ledger.Location)
}

func i64(v int64) *int64 { return &v }

func TestExpected(t *testing.T) {
	got := Expected(ModeSingle, 1600000, 0, "2026-10")
	want := []Payment{{Kind: KindFull, Amount: 1600000, Payday: d(2026, 10, 31), Note: "Зарплата"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("single = %+v", got)
	}
	got = Expected(ModeSplit, 1600000, 800000, "2026-10")
	want = []Payment{
		{Kind: KindAdvance, Amount: 800000, Payday: d(2026, 10, 15), Note: "Аванс"},
		{Kind: KindRest, Amount: 800000, Payday: d(2026, 10, 31), Note: "Зарплата (остаток)"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("split = %+v", got)
	}
	if got := Expected(ModeSplit, 1600000, 600000, "2026-10"); got[1].Amount != 1000000 {
		t.Errorf("rest = %d", got[1].Amount)
	}
}

func TestLastDay(t *testing.T) {
	for month, day := range map[string]int{"2027-02": 28, "2028-02": 29, "2026-04": 30, "2026-12": 31} {
		if got := LastDay(month); got.Day() != day {
			t.Errorf("LastDay(%s) = %d, want %d", month, got.Day(), day)
		}
	}
}

func TestEffectiveAdvance(t *testing.T) {
	cases := []struct {
		salary int64
		adv    *int64
		want   int64
	}{
		{1600000, nil, 800000},
		{1600050, nil, 800000},
		{1600000, i64(99), 800000},
		{1600000, i64(1599901), 800000},
		{1600000, i64(1599900), 1599900},
		{1600000, i64(600000), 600000},
	}
	for _, c := range cases {
		if got := EffectiveAdvance(c.salary, c.adv); got != c.want {
			t.Errorf("EffectiveAdvance(%d, %v) = %d, want %d", c.salary, c.adv, got, c.want)
		}
	}
}

func TestNextPayday(t *testing.T) {
	split := []struct {
		now  time.Time
		day  time.Time
		kind Kind
	}{
		{at(2026, 10, 10, 12), d(2026, 10, 15), KindAdvance},
		{at(2026, 10, 15, 9), d(2026, 10, 31), KindRest},
		{at(2026, 10, 16, 9), d(2026, 10, 31), KindRest},
		{at(2026, 10, 31, 9), d(2026, 11, 15), KindAdvance},
		{at(2028, 2, 28, 9), d(2028, 2, 29), KindRest},
		{at(2028, 2, 29, 9), d(2028, 3, 15), KindAdvance},
	}
	for _, c := range split {
		day, kind := NextPayday(c.now, ModeSplit)
		if !day.Equal(c.day) || kind != c.kind {
			t.Errorf("split %v → %v %s, want %v %s", c.now, day, kind, c.day, c.kind)
		}
	}
	if day, kind := NextPayday(at(2026, 10, 10, 9), ModeSingle); !day.Equal(d(2026, 10, 31)) || kind != KindFull {
		t.Errorf("single 10th → %v %s", day, kind)
	}
	if day, _ := NextPayday(at(2026, 10, 31, 9), ModeSingle); !day.Equal(d(2026, 11, 30)) {
		t.Errorf("single 31st → %v", day)
	}
}

func TestBudgetDays(t *testing.T) {
	if n := BudgetDays(at(2026, 10, 10, 23), d(2026, 10, 15)); n != 5 {
		t.Errorf("10th→15th = %d", n)
	}
	if n := BudgetDays(at(2026, 10, 30, 1), d(2026, 10, 31)); n != 1 {
		t.Errorf("30th→31st = %d", n)
	}
	if n := BudgetDays(at(2026, 10, 31, 1), d(2026, 10, 31)); n != 1 {
		t.Errorf("same day min = %d", n)
	}
}

func TestNotesAndIDs(t *testing.T) {
	if Note(KindAdvance) != "Аванс" || Note(KindRest) != "Зарплата (остаток)" || Note(KindFull) != "Зарплата" {
		t.Error("notes")
	}
	if TxID("2026-10", KindAdvance) != "p_2026-10_advance" || !IsPayoutTxID("p_2026-10_rest") || IsPayoutTxID("m_abc") {
		t.Error("ids")
	}
	if _, err := ParseMode("weekly"); err == nil {
		t.Error("ParseMode weekly")
	}
	if _, err := ParseKind("bonus"); err == nil {
		t.Error("ParseKind bonus")
	}
}
