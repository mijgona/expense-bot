package bot

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"expense-bot/internal/category"
	"expense-bot/internal/sheets"
)

// formatReport builds a Markdown report string for the given month key (e.g. "2026-03").
func formatReport(stats *sheets.MonthStats, salary int, monthKey string) string {
	t, _ := time.Parse("2006-01", monthKey)
	remaining := float64(salary) - stats.Total

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *Отчёт за %s %d*\n\n", ruMonth(t.Month()), t.Year()))
	sb.WriteString(fmt.Sprintf("💼 Зарплата: *%s с.*\n", fmtNum(float64(salary))))
	sb.WriteString(fmt.Sprintf("💸 Потрачено: *%s с.*\n", fmtNum(stats.Total)))
	sb.WriteString(fmt.Sprintf("💚 Остаток: *%s с.*\n", fmtNum(remaining)))

	if len(stats.Totals) == 0 {
		sb.WriteString("\n_Расходов за этот период нет_")
		return sb.String()
	}

	sb.WriteString("\n─────────────────\n")

	// Sort categories by amount descending.
	type entry struct {
		name string
		amt  float64
	}
	entries := make([]entry, 0, len(stats.Totals))
	for k, v := range stats.Totals {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].amt > entries[j].amt })

	for _, e := range entries {
		cat := category.FindByName(e.name)
		if cat == nil {
			sb.WriteString(fmt.Sprintf("• %s: *%s с.*\n", e.name, fmtNum(e.amt)))
			continue
		}
		pct := e.amt / float64(cat.Limit) * 100
		icon := statusIcon(pct)
		sb.WriteString(fmt.Sprintf("%s %s: *%s/%s с.* (%.0f%%)\n",
			icon, cat.Label, fmtNum(e.amt), fmtNum(float64(cat.Limit)), pct))
	}
	return sb.String()
}

// formatBalance builds a balance summary string.
func formatBalance(stats *sheets.MonthStats, salary int) string {
	now := time.Now()
	remaining := float64(salary) - stats.Total
	pct := stats.Total / float64(salary) * 100
	daysLeft := 30 - now.Day()
	if daysLeft < 1 {
		daysLeft = 1
	}
	daily := remaining / float64(daysLeft)

	return fmt.Sprintf(
		"%s *Баланс на %s*\n\n"+
			"💼 Зарплата: *%s с.*\n"+
			"💸 Потрачено: *%s с.* (%.1f%%)\n"+
			"💚 Остаток: *%s с.*\n\n"+
			"📅 Осталось дней: *%d*\n"+
			"📊 На день: *%s с.*",
		balanceIcon(remaining, float64(salary)),
		now.Format("02.01.2006"),
		fmtNum(float64(salary)),
		fmtNum(stats.Total), pct,
		fmtNum(remaining),
		daysLeft,
		fmtNum(daily),
	)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func statusIcon(pct float64) string {
	switch {
	case pct > 100:
		return "🔴"
	case pct > 80:
		return "🟡"
	default:
		return "🟢"
	}
}

func balanceIcon(remaining, salary float64) string {
	switch {
	case remaining < salary*0.2:
		return "🔴"
	case remaining < salary*0.5:
		return "🟡"
	default:
		return "🟢"
	}
}

func fmtNum(f float64) string {
	s := fmt.Sprintf("%.0f", f)
	n := len(s)
	if n <= 3 {
		return s
	}
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	return b.String()
}

func ruMonth(m time.Month) string {
	names := map[time.Month]string{
		time.January: "Январь", time.February: "Февраль", time.March: "Март",
		time.April: "Апрель", time.May: "Май", time.June: "Июнь",
		time.July: "Июль", time.August: "Август", time.September: "Сентябрь",
		time.October: "Октябрь", time.November: "Ноябрь", time.December: "Декабрь",
	}
	return names[m]
}
