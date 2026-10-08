package firestore

import (
	"testing"
	"time"

	"expense-bot/internal/store"
)

func TestShouldSkip(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name       string
		existing   *store.Transaction
		tombstoned bool
		want       bool
	}{
		{"new record", nil, false, false},
		{"untouched migrated record is overwritten", &store.Transaction{Source: "migration"}, false, false},
		{"edited by the user", &store.Transaction{Source: "migration", EditedAt: &now}, false, true},
		{"deleted by the user", nil, true, true},
	}
	for _, c := range cases {
		if got := ShouldSkip(c.existing, c.tombstoned); got != c.want {
			t.Errorf("%s: ShouldSkip = %v, want %v", c.name, got, c.want)
		}
	}
}
