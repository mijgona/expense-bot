package ledger

import "testing"

func ptr[T any](v T) *T { return &v }

func TestEffectiveDefaults(t *testing.T) {
	e := NewEffective("Алиса", 16000, ProfileOverrides{})
	if e.DisplayName != "Алиса" || !e.DisplayNameIsDefault || e.Salary != 1600000 || !e.SalaryIsDefault {
		t.Errorf("defaults = %+v", e)
	}
	if e.Limits["Еда"] != 200000 || !e.LimitIsDefault["Еда"] || len(e.Limits) != 14 {
		t.Errorf("limits = %+v", e.Limits)
	}
}

func TestEffectiveOverrides(t *testing.T) {
	e := NewEffective("Алиса", 16000, ProfileOverrides{
		DisplayName: ptr("Мижгона"),
		Salary:      ptr(int64(2000000)),
		Limits:      map[string]int64{"Еда": 300000, "Связь": 0, "Неизвестная": 5},
	})
	if e.DisplayName != "Мижгона" || e.DisplayNameIsDefault || e.Salary != 2000000 || e.SalaryIsDefault {
		t.Errorf("overrides = %+v", e)
	}
	if e.Limits["Еда"] != 300000 || e.LimitIsDefault["Еда"] || e.LimitDefaults["Еда"] != 200000 {
		t.Error("Еда override")
	}
	if e.Limits["Связь"] != 0 || e.LimitIsDefault["Связь"] {
		t.Error("Связь = 0 override")
	}
	if e.Limits["Транспорт"] != 100000 || !e.LimitIsDefault["Транспорт"] {
		t.Error("Транспорт default")
	}
	if _, ok := e.Limits["Неизвестная"]; ok {
		t.Error("unknown category leaked into limits")
	}
}

func TestEffectiveEmptyName(t *testing.T) {
	e := NewEffective("Алиса", 16000, ProfileOverrides{DisplayName: ptr("  ")})
	if e.DisplayName != "Алиса" || !e.DisplayNameIsDefault {
		t.Errorf("blank override = %+v", e)
	}
}
