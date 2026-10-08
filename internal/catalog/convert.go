package catalog

import (
	"time"

	"expense-bot/internal/category"
)

// Convert builds a user's category list for the one-time switch to category IDs (research R5).
// Existing entries are never overwritten (renames survive re-runs); missing defaults are added
// with the user's 005 limits (keyed by legacy name); every non-ID record value gets an entry
// (hidden for unknown names). It returns the list and the name→ID mapping for the values that
// need rewriting. The 50-entry cap is not enforced here: legacy data is kept whole.
func Convert(existing List, txValues []string, limits005 map[string]int64, now time.Time) (List, map[string]string) {
	l := existing.Clone()
	if l == nil {
		l = List{}
	}
	for i, c := range category.All() {
		id := DefaultID(c.Key)
		if _, ok := l[id]; ok {
			continue
		}
		e := Defaults(now)[id]
		if v, ok := limits005[c.Name]; ok {
			e.Limit = v
		}
		e.Position = i
		if len(existing) > 0 {
			e.Position = l.nextPosition()
		}
		l[id] = e
	}

	mapping := map[string]string{}
	for _, v := range txValues {
		if v == "" || IsID(v) {
			continue
		}
		id := l.Resolve(v)
		mapping[v] = id
		if _, ok := l[id]; !ok {
			l[id] = Entry{Name: v, NameKey: NormalizeKey(v), Hidden: true, Position: l.nextPosition(), CreatedAt: now}
		}
	}
	return l, mapping
}

// MapTotals re-keys per-category totals from legacy names to IDs, summing keys that collapse
// onto the same ID (used to verify the conversion against the before-snapshot).
func MapTotals(before map[string]int64, mapping map[string]string) map[string]int64 {
	out := map[string]int64{}
	for k, v := range before {
		if id, ok := mapping[k]; ok {
			k = id
		}
		out[k] += v
	}
	return out
}
