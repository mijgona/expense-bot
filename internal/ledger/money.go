package ledger

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Diram is the stored money unit: 1 с. = 100 дирам.
type Diram = int64

const (
	PerSomoni = 100
	MaxAmount = 1_000_000_000 // 10 000 000 с.
	MaxNote   = 200
)

// ValidateAmount checks a positive diram amount within limits.
func ValidateAmount(a int64) error {
	if a < 1 {
		return errors.New("Сумма должна быть больше нуля")
	}
	if a > MaxAmount {
		return errors.New("Сумма не может быть больше 10 000 000 с.")
	}
	return nil
}

// ValidateNote trims s and checks its length.
func ValidateNote(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > MaxNote {
		return "", fmt.Errorf("Описание не может быть длиннее %d символов", MaxNote)
	}
	return s, nil
}

// ParseSomoni parses a somoni amount as written in Sheets ("350", "350.5", "350,50",
// "1 234,5", "-350") into signed diram, rounding half away from zero.
func ParseSomoni(s string) (int64, error) {
	raw := s
	s = strings.NewReplacer(" ", "", " ", "", " ", "").Replace(strings.TrimSpace(s))
	if s == "" {
		return 0, fmt.Errorf("пустая сумма")
	}
	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg, s = true, s[1:]
	case strings.HasPrefix(s, "−"): // U+2212 minus, as Sheets may render it
		neg, s = true, strings.TrimPrefix(s, "−")
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	s = strings.ReplaceAll(s, ",", ".")
	intPart, frac, hasFrac := strings.Cut(s, ".")
	if intPart == "" || !isDigits(intPart) || (hasFrac && (frac == "" || !isDigits(frac))) {
		return 0, fmt.Errorf("неверная сумма %q", raw)
	}
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || whole > MaxAmount {
		return 0, fmt.Errorf("неверная сумма %q", raw)
	}
	cents := whole * PerSomoni
	if hasFrac {
		// first two digits are diram; the third decides rounding
		padded := frac + "00"
		d, _ := strconv.ParseInt(padded[:2], 10, 64)
		cents += d
		if len(frac) > 2 && frac[2] >= '5' {
			cents++
		}
	}
	if neg {
		cents = -cents
	}
	return cents, nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// FormatSomoni renders diram as "1 234 с." or "1 234,50 с." (for server-side messages).
func FormatSomoni(d int64) string {
	sign := ""
	if d < 0 {
		sign, d = "−", -d
	}
	whole, frac := d/PerSomoni, d%PerSomoni
	s := strconv.FormatInt(whole, 10)
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(ch)
	}
	if frac != 0 {
		fmt.Fprintf(&b, ",%02d", frac)
	}
	return sign + b.String() + " с."
}
