package payroll

import (
	"context"
	"log"
	"slices"
	"time"

	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

const (
	reminderHour = 10
	tickEvery    = 10 * time.Minute
)

// Source is the store subset the reminder needs.
type Source interface {
	ListUserIDs(ctx context.Context) ([]int64, error)
	GetUser(ctx context.Context, id int64) (store.User, error)
	ClaimReminder(ctx context.Context, userID int64, month, kind, txID string) (bool, error)
}

// Sender delivers a reminder; url opens the Mini App ("" = no button).
type Sender interface {
	Send(ctx context.Context, chatID int64, text, url string) error
}

// Reminder sends one bot message per due payment on paydays at 10:00 Dushanbe (007 FR-012).
type Reminder struct {
	src           Source
	send          Sender
	clock         func() time.Time
	webAppURL     string
	allowed       []int64
	salaryDefault int // somoni (cfg.Salary)
}

func NewReminder(src Source, send Sender, clock func() time.Time, webAppURL string, allowed []int64, salaryDefault int) *Reminder {
	return &Reminder{src: src, send: send, clock: clock, webAppURL: webAppURL, allowed: allowed, salaryDefault: salaryDefault}
}

// Start ticks every 10 minutes until ctx is done.
func (r *Reminder) Start(ctx context.Context) {
	log.Println("payroll: reminders started")
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	r.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.Tick(ctx)
		}
	}
}

func reminderText(p Payment) string {
	switch p.Kind {
	case KindAdvance:
		return "💵 Сегодня аванс. Запишите, сколько пришло."
	case KindRest:
		return "💵 Сегодня зарплата (остаток). Запишите, сколько пришло."
	default:
		return "💵 Сегодня зарплата. Запишите, сколько пришло."
	}
}

// Tick checks every user once. Claim-then-send guarantees at most one message per payment.
func (r *Reminder) Tick(ctx context.Context) {
	now := r.clock().In(ledger.Location)
	if now.Hour() < reminderHour {
		return
	}
	today := dayOf(now)
	month := ledger.MonthKey(today)
	if today.Day() != AdvanceDay && !today.Equal(LastDay(month)) {
		return
	}
	ids, err := r.src.ListUserIDs(ctx)
	if err != nil {
		log.Printf("payroll: list users: %v", err)
		return
	}
	for _, id := range ids {
		if len(r.allowed) > 0 && !slices.Contains(r.allowed, id) {
			continue
		}
		u, err := r.src.GetUser(ctx, id)
		if err != nil {
			log.Printf("payroll: user %d: %v", id, err)
			continue
		}
		if u.SalaryRemindersOff {
			continue
		}
		for _, p := range ExpectedFor(u, r.salaryDefault, month) {
			if !p.Payday.Equal(today) {
				continue
			}
			ok, err := r.src.ClaimReminder(ctx, id, month, string(p.Kind), TxID(month, p.Kind))
			if err != nil {
				log.Printf("payroll: claim %d %s: %v", id, p.Kind, err)
				continue
			}
			if !ok {
				continue
			}
			url := ""
			if r.webAppURL != "" {
				url = r.webAppURL + "?payout=" + month + "_" + string(p.Kind)
			}
			if err := r.send.Send(ctx, id, reminderText(p), url); err != nil {
				log.Printf("payroll: send to %d: %v", id, err) // no retry: claim already taken
			}
		}
	}
}
