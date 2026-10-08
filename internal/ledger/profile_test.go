package ledger

import "testing"

func ptr[T any](v T) *T { return &v }

func TestEffectiveDefaults(t *testing.T) {
	e := NewEffective("Алиса", 16000, ProfileOverrides{})
	if e.DisplayName != "Алиса" || !e.DisplayNameIsDefault || e.Salary != 1600000 || !e.SalaryIsDefault {
		t.Errorf("defaults = %+v", e)
	}
}

func TestEffectiveOverrides(t *testing.T) {
	e := NewEffective("Алиса", 16000, ProfileOverrides{DisplayName: ptr("Мижгона"), Salary: ptr(int64(2000000))})
	if e.DisplayName != "Мижгона" || e.DisplayNameIsDefault || e.Salary != 2000000 || e.SalaryIsDefault {
		t.Errorf("overrides = %+v", e)
	}
}

func TestEffectiveEmptyName(t *testing.T) {
	e := NewEffective("Алиса", 16000, ProfileOverrides{DisplayName: ptr("  ")})
	if e.DisplayName != "Алиса" || !e.DisplayNameIsDefault {
		t.Errorf("blank override = %+v", e)
	}
}
