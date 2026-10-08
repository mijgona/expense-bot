package catalog

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"expense-bot/internal/category"
	"expense-bot/internal/ledger"
)

const (
	MaxEntries = 50
	MaxName    = 30
)

var (
	ErrDuplicate    = errors.New("category name already exists")
	ErrInvalidName  = errors.New("category name must be 1–30 characters")
	ErrLimitReached = errors.New("too many categories")
	ErrLastVisible  = errors.New("at least one category must stay visible")
	ErrBadOrder     = errors.New("order must list every category exactly once")
	ErrInvalidLimit = errors.New("limit out of range")
	ErrNotFound     = errors.New("category not found")
)

// Entry is one category of a user (stored in users/{id}.categories[<id>]).
type Entry struct {
	Name       string    `firestore:"name"`
	NameKey    string    `firestore:"nameKey"`
	Limit      int64     `firestore:"limit"` // diram; 0 = no limit
	Hidden     bool      `firestore:"hidden"`
	Position   int       `firestore:"position"`
	DefaultKey string    `firestore:"defaultKey,omitempty"`
	CreatedAt  time.Time `firestore:"createdAt"`
}

// List is a user's categories keyed by ID. Entries are never removed.
type List map[string]Entry

// View is an entry with its ID.
type View struct {
	ID string
	Entry
}

// Defaults returns the 14 default categories (FR-015, FR-018).
func Defaults(now time.Time) List {
	l := List{}
	for i, c := range category.All() {
		l[DefaultID(c.Key)] = Entry{
			Name: c.Label, NameKey: NormalizeKey(c.Label), Limit: int64(c.Limit) * ledger.PerSomoni,
			Position: i, DefaultKey: c.Key, CreatedAt: now,
		}
	}
	return l
}

// Clone returns a copy that can be modified safely.
func (l List) Clone() List {
	out := make(List, len(l))
	for k, v := range l {
		out[k] = v
	}
	return out
}

func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > MaxName {
		return "", ErrInvalidName
	}
	return name, nil
}

func validLimit(limit int64) error {
	if limit < 0 || limit > ledger.MaxAmount {
		return ErrInvalidLimit
	}
	return nil
}

func (l List) nameTaken(key, exceptID string) bool {
	for id, e := range l {
		if id != exceptID && e.NameKey == key {
			return true
		}
	}
	return false
}

func (l List) nextPosition() int {
	max := -1
	for _, e := range l {
		if e.Position > max {
			max = e.Position
		}
	}
	return max + 1
}

// Add appends a new visible category.
func (l List) Add(id, name string, limit int64, now time.Time) error {
	name, err := validName(name)
	if err != nil {
		return err
	}
	if err := validLimit(limit); err != nil {
		return err
	}
	if len(l) >= MaxEntries {
		return ErrLimitReached
	}
	key := NormalizeKey(name)
	if l.nameTaken(key, "") {
		return ErrDuplicate
	}
	l[id] = Entry{Name: name, NameKey: key, Limit: limit, Position: l.nextPosition(), CreatedAt: now}
	return nil
}

// Rename changes the display name; ID, limit and position stay.
func (l List) Rename(id, name string) error {
	e, ok := l[id]
	if !ok {
		return ErrNotFound
	}
	name, err := validName(name)
	if err != nil {
		return err
	}
	key := NormalizeKey(name)
	if l.nameTaken(key, id) {
		return ErrDuplicate
	}
	e.Name, e.NameKey = name, key
	l[id] = e
	return nil
}

// SetLimit changes the monthly limit (0 = none).
func (l List) SetLimit(id string, limit int64) error {
	e, ok := l[id]
	if !ok {
		return ErrNotFound
	}
	if err := validLimit(limit); err != nil {
		return err
	}
	e.Limit = limit
	l[id] = e
	return nil
}

// SetHidden hides or shows a category; the last visible one cannot be hidden.
func (l List) SetHidden(id string, hidden bool) error {
	e, ok := l[id]
	if !ok {
		return ErrNotFound
	}
	if hidden && !e.Hidden && len(l.Visible()) <= 1 {
		return ErrLastVisible
	}
	e.Hidden = hidden
	l[id] = e
	return nil
}

// Reorder sets positions 0..n-1 in the given order; ids must be a permutation of all IDs.
func (l List) Reorder(ids []string) error {
	if len(ids) != len(l) {
		return ErrBadOrder
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if _, ok := l[id]; !ok || seen[id] {
			return ErrBadOrder
		}
		seen[id] = true
	}
	for i, id := range ids {
		e := l[id]
		e.Position = i
		l[id] = e
	}
	return nil
}

// Get returns one entry.
func (l List) Get(id string) (Entry, bool) {
	e, ok := l[id]
	return e, ok
}

// Sorted returns all entries by position (ties by ID).
func (l List) Sorted() []View {
	out := make([]View, 0, len(l))
	for id, e := range l {
		out = append(out, View{ID: id, Entry: e})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position != out[j].Position {
			return out[i].Position < out[j].Position
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Visible returns non-hidden entries by position.
func (l List) Visible() []View {
	var out []View
	for _, v := range l.Sorted() {
		if !v.Hidden {
			out = append(out, v)
		}
	}
	return out
}

// Usable reports whether id may be used for a new record or a category change.
func (l List) Usable(id string) bool {
	e, ok := l[id]
	return ok && !e.Hidden
}

// Resolve maps a record's category value to an ID: IDs pass through, default legacy names map
// to their default ID, anything else to its legacy ID.
func (l List) Resolve(value string) string {
	if IsID(value) {
		return value
	}
	if c := category.ByName(value); c != nil {
		return DefaultID(c.Key)
	}
	return LegacyID(value)
}
