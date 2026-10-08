package catalog

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func TestNormalizeKey(t *testing.T) {
	if NormalizeKey("  Еда  ") != NormalizeKey("еда") {
		t.Error("case/space not normalised")
	}
	if NormalizeKey("Еда   и  Продукты") != "еда и продукты" {
		t.Errorf("collapse = %q", NormalizeKey("Еда   и  Продукты"))
	}
	if NormalizeKey("🐱 Кошка") != "🐱 кошка" {
		t.Error("emoji lost")
	}
}

func TestIDs(t *testing.T) {
	if DefaultID("food") != "c_food" {
		t.Error("DefaultID")
	}
	a, b := LegacyID("Без категории"), LegacyID("Без категории")
	if a != b || !strings.HasPrefix(a, "c_l") || len(a) != 13 {
		t.Errorf("LegacyID = %q", a)
	}
	a1, a2 := ClientID("3f2a9b1c-0d4e-4a1b-8c2d-123456789abc"), ClientID("3F2A9B1C-0D4E-4A1B-8C2D-123456789ABC")
	b1 := ClientID("3f2a9b1c-0d4e-4a1b-8c2d-000000000000")
	if a1 != a2 || a1 == b1 || !strings.HasPrefix(a1, "c_") || len(a1) != 14 {
		t.Errorf("ClientID = %q / %q / %q", a1, a2, b1)
	}
	if !IsID("c_food") || IsID("Еда") {
		t.Error("IsID")
	}
}

func TestAddRules(t *testing.T) {
	l := Defaults(now)
	if err := l.Add("c_cat", "🐱 Кошка", 30000, now); err != nil {
		t.Fatal(err)
	}
	if e, _ := l.Get("c_cat"); e.Position != 14 {
		t.Errorf("position = %d, want 14", e.Position)
	}
	if err := l.Add("c_x", " 🐱 кошка ", 0, now); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate: %v", err)
	}
	if err := l.Add("c_y", "   ", 0, now); !errors.Is(err, ErrInvalidName) {
		t.Errorf("empty: %v", err)
	}
	if err := l.Add("c_y", strings.Repeat("я", 31), 0, now); !errors.Is(err, ErrInvalidName) {
		t.Errorf("31 runes: %v", err)
	}
	if err := l.Add("c_y", "ok", -1, now); !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("limit: %v", err)
	}
	for i := len(l); i < MaxEntries; i++ {
		if err := l.Add(fmt.Sprintf("c_n%d", i), fmt.Sprintf("n%d", i), 0, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Add("c_over", "over", 0, now); !errors.Is(err, ErrLimitReached) {
		t.Errorf("51st: %v", err)
	}
}

func TestRename(t *testing.T) {
	l := Defaults(now)
	if err := l.Rename("c_transport", "🍽 Еда/Продукты"); !errors.Is(err, ErrDuplicate) {
		t.Errorf("rename to existing: %v", err)
	}
	if err := l.Rename("c_food", "🍽 ЕДА/продукты"); err != nil {
		t.Errorf("own name different case: %v", err)
	}
	if err := l.Rename("c_nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	if err := l.Rename("c_transport", "🚕 Такси"); err != nil {
		t.Fatal(err)
	}
	if e, _ := l.Get("c_transport"); e.Name != "🚕 Такси" || e.Limit != 100000 || e.Position != 2 {
		t.Errorf("after rename = %+v", e)
	}
}

func TestHideLastVisible(t *testing.T) {
	l := Defaults(now)
	ids := []string{}
	for _, v := range l.Sorted() {
		ids = append(ids, v.ID)
	}
	for _, id := range ids[1:] {
		if err := l.SetHidden(id, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.SetHidden(ids[0], true); !errors.Is(err, ErrLastVisible) {
		t.Errorf("last visible: %v", err)
	}
	if len(l.Visible()) != 1 {
		t.Errorf("visible = %d", len(l.Visible()))
	}
	if err := l.SetHidden(ids[1], false); err != nil || len(l.Visible()) != 2 {
		t.Errorf("show again: %v", err)
	}
}

func TestReorder(t *testing.T) {
	l := Defaults(now)
	var ids []string
	for _, v := range l.Sorted() {
		ids = append([]string{v.ID}, ids...) // reversed
	}
	if err := l.Reorder(ids); err != nil {
		t.Fatal(err)
	}
	for i, v := range l.Sorted() {
		if v.ID != ids[i] || v.Position != i {
			t.Fatalf("pos %d = %s/%d", i, v.ID, v.Position)
		}
	}
	if err := l.Reorder(ids[1:]); !errors.Is(err, ErrBadOrder) {
		t.Errorf("missing id: %v", err)
	}
	if err := l.Reorder(append(ids[:13:13], ids[0])); !errors.Is(err, ErrBadOrder) {
		t.Errorf("duplicate id: %v", err)
	}
}

func TestSetLimitAndResolve(t *testing.T) {
	l := Defaults(now)
	if err := l.SetLimit("c_food", -5); !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("negative: %v", err)
	}
	if err := l.SetLimit("c_food", 0); err != nil {
		t.Error(err)
	}
	if l.Resolve("c_food") != "c_food" || l.Resolve("Еда") != "c_food" || l.Resolve("Старое") != LegacyID("Старое") {
		t.Error("Resolve")
	}
}
