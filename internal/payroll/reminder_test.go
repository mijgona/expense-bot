package payroll

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"expense-bot/internal/store"
)

type fakeSrc struct {
	users   map[int64]store.User
	claimed map[string]bool // uid|month|kind
	blocked map[string]bool // recorded or dismissed
}

func (f *fakeSrc) ListUserIDs(context.Context) ([]int64, error) {
	var ids []int64
	for id := range f.users {
		ids = append(ids, id)
	}
	return ids, nil
}
func (f *fakeSrc) GetUser(_ context.Context, id int64) (store.User, error) { return f.users[id], nil }
func (f *fakeSrc) ClaimReminder(_ context.Context, uid int64, month, kind, _ string) (bool, error) {
	key := month + "|" + kind
	ukey := fmt.Sprint(uid) + "|" + key
	if f.blocked[key] || f.claimed[ukey] {
		return false, nil
	}
	f.claimed[ukey] = true
	return true, nil
}

type sent struct {
	chat      int64
	text, url string
}

type fakeSender struct {
	msgs []sent
	fail bool
}

func (s *fakeSender) Send(_ context.Context, chat int64, text, url string) error {
	s.msgs = append(s.msgs, sent{chat, text, url})
	if s.fail {
		return errors.New("bot was blocked by the user")
	}
	return nil
}

func setup(mode string, at time.Time) (*fakeSrc, *fakeSender, *Reminder) {
	src := &fakeSrc{users: map[int64]store.User{42: {ID: 42, FirstName: "A", SalaryMode: mode}},
		claimed: map[string]bool{}, blocked: map[string]bool{}}
	snd := &fakeSender{}
	r := NewReminder(src, snd, func() time.Time { return at }, "https://app.example", nil, 16000)
	return src, snd, r
}

func TestReminderTiming(t *testing.T) {
	for _, now := range []time.Time{at(2026, 10, 14, 12), at(2026, 10, 15, 9)} {
		_, snd, r := setup("split", now)
		r.Tick(context.Background())
		if len(snd.msgs) != 0 {
			t.Errorf("%v: %d messages, want 0", now, len(snd.msgs))
		}
	}
}

func TestReminderAdvanceOnce(t *testing.T) {
	_, snd, r := setup("split", time.Date(2026, 10, 15, 10, 5, 0, 0, d(2026, 1, 1).Location()))
	r.Tick(context.Background())
	if len(snd.msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(snd.msgs))
	}
	m := snd.msgs[0]
	if m.chat != 42 || !strings.Contains(m.text, "аванс") || !strings.Contains(m.text, "8 000") || m.url != "https://app.example?payout=2026-10_advance" {
		t.Errorf("message = %+v", m)
	}
	r.Tick(context.Background())
	if len(snd.msgs) != 1 {
		t.Errorf("second tick sent again: %d", len(snd.msgs))
	}
}

func TestReminderSkips(t *testing.T) {
	now := at(2026, 10, 15, 11)
	src, snd, r := setup("split", now)
	src.blocked["2026-10|advance"] = true // recorded or dismissed
	r.Tick(context.Background())
	if len(snd.msgs) != 0 {
		t.Error("sent for recorded/dismissed payment")
	}

	src, snd, r = setup("split", now)
	u := src.users[42]
	u.SalaryRemindersOff = true
	src.users[42] = u
	r.Tick(context.Background())
	if len(snd.msgs) != 0 {
		t.Error("sent with reminders off")
	}

	src, snd, _ = setup("split", now)
	r = NewReminder(src, snd, func() time.Time { return now }, "", []int64{7}, 16000)
	r.Tick(context.Background())
	if len(snd.msgs) != 0 {
		t.Error("sent to user outside allowed list")
	}
}

func TestReminderSendFailureContinues(t *testing.T) {
	now := at(2026, 10, 15, 11)
	src, snd, r := setup("split", now)
	src.users[43] = store.User{ID: 43, SalaryMode: "split"}
	snd.fail = true
	r.Tick(context.Background())
	if len(snd.msgs) != 2 {
		t.Errorf("attempts = %d, want 2 (failure must not stop the loop)", len(snd.msgs))
	}
}

func TestReminderSingleLastDayAndNoURL(t *testing.T) {
	src, snd, _ := setup("", at(2026, 10, 31, 10))
	r := NewReminder(src, snd, func() time.Time { return at(2026, 10, 31, 10) }, "", nil, 16000)
	r.Tick(context.Background())
	if len(snd.msgs) != 1 || !strings.Contains(snd.msgs[0].text, "Сегодня зарплата: 16 000") || snd.msgs[0].url != "" {
		t.Errorf("single last day = %+v", snd.msgs)
	}
}
