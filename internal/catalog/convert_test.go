package catalog

import (
	"fmt"
	"reflect"
	"testing"
)

func TestConvertFromEmpty(t *testing.T) {
	l, mapping := Convert(nil, []string{"Еда", "Без категории", "Транспорт", "c_food"}, map[string]int64{"Еда": 300000}, now)
	if len(l) != 15 {
		t.Fatalf("entries = %d, want 14 defaults + 1 legacy", len(l))
	}
	food, _ := l.Get("c_food")
	if food.Name != "🍽 Еда/Продукты" || food.Limit != 300000 || food.Position != 0 || food.Hidden || food.DefaultKey != "food" {
		t.Errorf("food = %+v", food)
	}
	if tr, _ := l.Get("c_transport"); tr.Limit != 100000 {
		t.Errorf("transport default limit = %d", tr.Limit)
	}
	leg, ok := l.Get(LegacyID("Без категории"))
	if !ok || !leg.Hidden || leg.Name != "Без категории" || leg.Position != 14 {
		t.Errorf("legacy = %+v ok=%v", leg, ok)
	}
	want := map[string]string{"Еда": "c_food", "Транспорт": "c_transport", "Без категории": LegacyID("Без категории")}
	if !reflect.DeepEqual(mapping, want) {
		t.Errorf("mapping = %v", mapping)
	}
}

func TestConvertKeepsRenamesAndIsIdempotent(t *testing.T) {
	existing := Defaults(now)
	_ = existing.Rename("c_food", "🛒 Продукты")
	l, _ := Convert(existing, []string{"Еда"}, nil, now)
	if f, _ := l.Get("c_food"); f.Name != "🛒 Продукты" {
		t.Errorf("rename lost: %q", f.Name)
	}
	again, mapping := Convert(l, []string{"c_food", "c_transport"}, nil, now)
	if len(mapping) != 0 || !reflect.DeepEqual(again, l) {
		t.Errorf("not idempotent: mapping=%v", mapping)
	}
}

func TestConvertManyLegacy(t *testing.T) {
	var vals []string
	for i := 0; i < 60; i++ {
		vals = append(vals, fmt.Sprintf("legacy-%d", i))
	}
	l, _ := Convert(nil, vals, nil, now)
	if len(l) != 74 {
		t.Errorf("entries = %d, want 74 (conversion may exceed the cap)", len(l))
	}
}

func TestMapTotals(t *testing.T) {
	before := map[string]int64{"Еда": 1000, "c_food": 500, "Старое": 70, "c_transport": 30}
	got := MapTotals(before, map[string]string{"Еда": "c_food", "Старое": LegacyID("Старое")})
	want := map[string]int64{"c_food": 1500, LegacyID("Старое"): 70, "c_transport": 30}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MapTotals = %v, want %v", got, want)
	}
}
