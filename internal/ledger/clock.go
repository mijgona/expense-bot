package ledger

import (
	"fmt"
	"regexp"
	"time"
	_ "time/tzdata" // embed zone data so Asia/Dushanbe works on any base image
)

// Location is the business time zone: every date, time and month key is computed here.
var Location = mustLoad("Asia/Dushanbe")

// nowFunc is overridable in tests.
var nowFunc = time.Now

var monthRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(fmt.Sprintf("ledger: load location %s: %v", name, err))
	}
	return loc
}

// SetNowForTest replaces the clock (tests only) and returns a restore function.
func SetNowForTest(f func() time.Time) (restore func()) {
	prev := nowFunc
	nowFunc = f
	return func() { nowFunc = prev }
}

// Now returns the current time in Asia/Dushanbe.
func Now() time.Time { return nowFunc().In(Location) }

// MonthKey returns "2006-01" for t in Asia/Dushanbe.
func MonthKey(t time.Time) string { return t.In(Location).Format("2006-01") }

// ParseMonth validates a "YYYY-MM" key and returns the first instant of that month.
func ParseMonth(s string) (time.Time, error) {
	if !monthRe.MatchString(s) {
		return time.Time{}, fmt.Errorf("неверный месяц %q, ожидается ГГГГ-ММ", s)
	}
	return time.ParseInLocation("2006-01", s, Location)
}

// PrevMonth returns the month key before m (m must be valid).
func PrevMonth(m string) string {
	t, _ := ParseMonth(m)
	return t.AddDate(0, -1, 0).Format("2006-01")
}

// NextMonth returns the month key after m (m must be valid).
func NextMonth(m string) string {
	t, _ := ParseMonth(m)
	return t.AddDate(0, 1, 0).Format("2006-01")
}

// DaysInMonth returns the number of days in t's month.
func DaysInMonth(t time.Time) int {
	t = t.In(Location)
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, Location).Day()
}

// Quarter returns "Q4 2026" for t.
func Quarter(t time.Time) string {
	t = t.In(Location)
	return fmt.Sprintf("Q%d %d", (int(t.Month())-1)/3+1, t.Year())
}

// AllowedQuarters returns the current quarter followed by the next three.
func AllowedQuarters(now time.Time) []string {
	now = now.In(Location)
	first := time.Date(now.Year(), now.Month()-(now.Month()-1)%3, 1, 0, 0, 0, 0, Location)
	out := make([]string, 4)
	for i := range out {
		out[i] = Quarter(first.AddDate(0, 3*i, 0))
	}
	return out
}
