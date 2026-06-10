package sheets

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const usersSheet = "Пользователи"

// Expense is the data written to a single row in Google Sheets.
// IsIncome=true writes a positive amount under category "Приход".
// IsIncome=false writes a negative amount under the given Category.
type Expense struct {
	Category    string
	Amount      float64 // always positive; sign is set by IsIncome
	Description string
	IsIncome    bool
}

// MonthStats holds aggregated data for a month.
type MonthStats struct {
	Totals       map[string]float64 // expense totals per category (positive values)
	TotalExpense float64
	TotalIncome  float64
}

// Client wraps the Google Sheets API for expense tracking.
type Client struct {
	svc           *sheets.Service
	spreadsheetID string
}

// New creates a Client authenticated via a service-account credentials file.
func New(credentialsFile, spreadsheetID string) (*Client, error) {
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials file: %w", err)
	}
	return NewFromJSON(data, spreadsheetID)
}

// NewFromJSON creates a Client from raw service-account JSON bytes.
// Use this when credentials come from an environment variable (e.g. Render).
func NewFromJSON(credentialsJSON []byte, spreadsheetID string) (*Client, error) {
	conf, err := google.JWTConfigFromJSON(credentialsJSON, sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	svc, err := sheets.NewService(context.Background(), option.WithTokenSource(conf.TokenSource(context.Background())))
	if err != nil {
		return nil, fmt.Errorf("create sheets service: %w", err)
	}

	return &Client{svc: svc, spreadsheetID: spreadsheetID}, nil
}

// AppendExpense writes one expense row to the user's personal sheet.
// The sheet is named after the user's Telegram ID and created with headers if absent.
func (c *Client) AppendExpense(userID int64, e Expense) (string, error) {
	sheetName := strconv.FormatInt(userID, 10)
	headers := []interface{}{"Дата", "Время", "Категория", "Сумма", "Описание", "Месяц"}
	if err := c.ensureSheet(sheetName, headers); err != nil {
		return "", fmt.Errorf("ensure sheet: %w", err)
	}

	now := time.Now()
	cat := e.Category
	amt := -e.Amount
	if e.IsIncome {
		cat = "Приход"
		amt = e.Amount
	}
	row := []interface{}{
		now.Format("02.01.2006"),
		now.Format("15:04"),
		cat,
		amt,
		e.Description,
		now.Format("2006-01"),
	}

	_, err := c.svc.Spreadsheets.Values.
		Append(c.spreadsheetID, sheetName+"!A1", &sheets.ValueRange{Values: [][]interface{}{row}}).
		ValueInputOption("USER_ENTERED").
		Do()
	if err != nil {
		return "", fmt.Errorf("append row: %w", err)
	}

	return sheetName, nil
}

// GetMonthStats reads the user's personal sheet and returns per-category totals
// for the given month key (e.g. "2026-03").
func (c *Client) GetMonthStats(userID int64, monthKey string) (*MonthStats, error) {
	sheetName := strconv.FormatInt(userID, 10)

	resp, err := c.svc.Spreadsheets.Values.
		Get(c.spreadsheetID, sheetName+"!A2:F100000").
		Do()
	if err != nil {
		// Sheet may not exist yet — return empty stats.
		return &MonthStats{Totals: map[string]float64{}}, nil
	}

	stats := &MonthStats{Totals: map[string]float64{}}
	for _, row := range resp.Values {
		if len(row) < 6 {
			continue
		}
		if fmt.Sprint(row[5]) != monthKey {
			continue
		}
		cat, _ := row[2].(string)
		amt, err := parseAmount(row[3])
		if err != nil || cat == "" {
			continue
		}
		if amt >= 0 {
			stats.TotalIncome += amt
		} else {
			stats.Totals[cat] += -amt
			stats.TotalExpense += -amt
		}
	}
	return stats, nil
}

// AddSaving writes one entry to the user's dedicated savings sheet.
// Sheet name: "<userID>_savings", columns: Дата | Время | Сумма | Описание | Месяц.
func (c *Client) AddSaving(userID int64, amount float64, desc string) error {
	sheetName := savingsSheetName(userID)
	headers := []interface{}{"Дата", "Время", "Сумма", "Описание", "Месяц"}
	if err := c.ensureSheet(sheetName, headers); err != nil {
		return fmt.Errorf("ensure savings sheet: %w", err)
	}
	now := time.Now()
	row := []interface{}{
		now.Format("02.01.2006"),
		now.Format("15:04"),
		amount,
		desc,
		now.Format("2006-01"),
	}
	_, err := c.svc.Spreadsheets.Values.
		Append(c.spreadsheetID, sheetName+"!A1", &sheets.ValueRange{Values: [][]interface{}{row}}).
		ValueInputOption("USER_ENTERED").
		Do()
	return err
}

// WithdrawSaving writes a negative entry to the savings sheet (reduces balance).
func (c *Client) WithdrawSaving(userID int64, amount float64, desc string) error {
	return c.AddSaving(userID, -amount, desc)
}

// GetSavingsBalance returns the all-time total of the user's savings.
func (c *Client) GetSavingsBalance(userID int64) (float64, error) {
	return c.sumSavingsCol(userID, "")
}

// GetMonthlySavings returns the total savings added in the given month key (e.g. "2026-03").
func (c *Client) GetMonthlySavings(userID int64, monthKey string) (float64, error) {
	return c.sumSavingsCol(userID, monthKey)
}

// sumSavingsCol reads the savings sheet and sums col C (Сумма).
// If monthKey is non-empty, only rows matching col E (Месяц) are counted.
func (c *Client) sumSavingsCol(userID int64, monthKey string) (float64, error) {
	resp, err := c.svc.Spreadsheets.Values.
		Get(c.spreadsheetID, savingsSheetName(userID)+"!A2:E100000").
		Do()
	if err != nil {
		return 0, nil // sheet doesn't exist yet
	}
	var total float64
	for _, row := range resp.Values {
		if len(row) < 3 {
			continue
		}
		if monthKey != "" && (len(row) < 5 || fmt.Sprint(row[4]) != monthKey) {
			continue
		}
		amt, err := parseAmount(row[2])
		if err != nil {
			continue
		}
		total += amt
	}
	return total, nil
}

func savingsSheetName(userID int64) string {
	return strconv.FormatInt(userID, 10) + "_savings"
}

func creditSheetName(userID int64) string {
	return strconv.FormatInt(userID, 10) + "_credit"
}

// AppendCreditExpense writes a positive-amount row to the user's _credit sheet.
// Columns: Дата | Время | Категория | Сумма | Описание | Месяц.
func (c *Client) AppendCreditExpense(userID int64, e Expense) error {
	sheetName := creditSheetName(userID)
	headers := []interface{}{"Дата", "Время", "Категория", "Сумма", "Описание", "Месяц"}
	if err := c.ensureSheet(sheetName, headers); err != nil {
		return fmt.Errorf("ensure credit sheet: %w", err)
	}
	now := time.Now()
	row := []interface{}{
		now.Format("02.01.2006"),
		now.Format("15:04"),
		e.Category,
		e.Amount, // positive: debt increases
		e.Description,
		now.Format("2006-01"),
	}
	_, err := c.svc.Spreadsheets.Values.
		Append(c.spreadsheetID, sheetName+"!A1", &sheets.ValueRange{Values: [][]interface{}{row}}).
		ValueInputOption("USER_ENTERED").
		Do()
	return err
}

// AddCreditRepayment records a repayment: negative row in _credit sheet (debt decreases)
// and a cash outflow row in the main sheet (category "Погашение кредита").
func (c *Client) AddCreditRepayment(userID int64, amount float64, desc string) error {
	sheetName := creditSheetName(userID)
	headers := []interface{}{"Дата", "Время", "Категория", "Сумма", "Описание", "Месяц"}
	if err := c.ensureSheet(sheetName, headers); err != nil {
		return fmt.Errorf("ensure credit sheet: %w", err)
	}
	now := time.Now()
	row := []interface{}{
		now.Format("02.01.2006"),
		now.Format("15:04"),
		"Погашение",
		-amount, // negative: debt decreases
		desc,
		now.Format("2006-01"),
	}
	if _, err := c.svc.Spreadsheets.Values.
		Append(c.spreadsheetID, sheetName+"!A1", &sheets.ValueRange{Values: [][]interface{}{row}}).
		ValueInputOption("USER_ENTERED").
		Do(); err != nil {
		return err
	}
	// Also record cash outflow in main sheet so GetCarryOver stays correct.
	_, err := c.AppendExpense(userID, Expense{
		Category:    "Погашение кредита",
		Amount:      amount,
		Description: desc,
	})
	return err
}

// GetCreditBalance returns the net outstanding credit balance (all-time).
// Positive = debt; zero when sheet is missing.
func (c *Client) GetCreditBalance(userID int64) (float64, error) {
	resp, err := c.svc.Spreadsheets.Values.
		Get(c.spreadsheetID, creditSheetName(userID)+"!D2:D100000").
		Do()
	if err != nil {
		return 0, nil // sheet doesn't exist yet
	}
	var total float64
	for _, row := range resp.Values {
		if len(row) == 0 {
			continue
		}
		amt, err := parseAmount(row[0])
		if err != nil {
			continue
		}
		total += amt
	}
	return total, nil
}

// GetMonthlyCreditStats returns charged and repaid totals for the given month key.
func (c *Client) GetMonthlyCreditStats(userID int64, monthKey string) (charged, repaid float64, err error) {
	resp, err := c.svc.Spreadsheets.Values.
		Get(c.spreadsheetID, creditSheetName(userID)+"!A2:F100000").
		Do()
	if err != nil {
		return 0, 0, nil // sheet doesn't exist yet
	}
	for _, row := range resp.Values {
		if len(row) < 6 {
			continue
		}
		if fmt.Sprint(row[5]) != monthKey {
			continue
		}
		amt, err := parseAmount(row[3])
		if err != nil {
			continue
		}
		if amt > 0 {
			charged += amt
		} else {
			repaid += -amt
		}
	}
	return charged, repaid, nil
}

// EnsureUser registers the user in the "Пользователи" sheet if not already present.
func (c *Client) EnsureUser(userID int64, firstName, username string) error {
	if err := c.ensureSheet(usersSheet, []interface{}{"UserID", "Имя", "Username", "Дата регистрации"}); err != nil {
		return err
	}

	resp, err := c.svc.Spreadsheets.Values.
		Get(c.spreadsheetID, usersSheet+"!A2:A10000").
		Do()
	if err != nil {
		return fmt.Errorf("read users sheet: %w", err)
	}

	idStr := strconv.FormatInt(userID, 10)
	for _, row := range resp.Values {
		if len(row) > 0 && fmt.Sprint(row[0]) == idStr {
			return nil // already registered
		}
	}

	row := []interface{}{idStr, firstName, username, time.Now().Format("02.01.2006 15:04")}
	_, err = c.svc.Spreadsheets.Values.
		Append(c.spreadsheetID, usersSheet+"!A1", &sheets.ValueRange{Values: [][]interface{}{row}}).
		ValueInputOption("USER_ENTERED").
		Do()
	return err
}

// GetCarryOver returns the net cash balance accumulated across all months
// strictly before upToMonthKey (e.g. "2026-04").
// Formula: Σ signed main-sheet amounts − Σ savings amounts for those months.
func (c *Client) GetCarryOver(userID int64, upToMonthKey string) (float64, error) {
	sheetName := strconv.FormatInt(userID, 10)

	var total float64

	resp, err := c.svc.Spreadsheets.Values.
		Get(c.spreadsheetID, sheetName+"!A2:F100000").
		Do()
	if err == nil {
		for _, row := range resp.Values {
			if len(row) < 6 {
				continue
			}
			if fmt.Sprint(row[5]) >= upToMonthKey {
				continue
			}
			amt, err := parseAmount(row[3])
			if err != nil {
				continue
			}
			total += amt // income positive, expenses negative
		}
	}

	savResp, err := c.svc.Spreadsheets.Values.
		Get(c.spreadsheetID, savingsSheetName(userID)+"!A2:E100000").
		Do()
	if err == nil {
		for _, row := range savResp.Values {
			if len(row) < 5 {
				continue
			}
			if fmt.Sprint(row[4]) >= upToMonthKey {
				continue
			}
			amt, err := parseAmount(row[2])
			if err != nil {
				continue
			}
			total -= amt // savings deposit reduces liquid balance
		}
	}

	return total, nil
}

// Goal represents a user-defined quarterly savings goal.
// Sheet columns: Название | Целевая сумма | Квартал | Статус | Описание
type Goal struct {
	Name         string
	TargetAmount float64
	Quarter      string // e.g. "Q3 2026"
	Status       string // "Активна" / "Выполнена"
	Description  string
}

// GetGoals reads goals from the user's "{userID}_goals" sheet.
// Returns nil slice if the sheet doesn't exist yet.
func (c *Client) GetGoals(userID int64) ([]Goal, error) {
	sheetName := strconv.FormatInt(userID, 10) + "_goals"
	resp, err := c.svc.Spreadsheets.Values.
		Get(c.spreadsheetID, sheetName+"!A2:E1000").
		Do()
	if err != nil {
		return nil, nil // sheet doesn't exist yet
	}
	var goals []Goal
	for _, row := range resp.Values {
		if len(row) < 1 {
			continue
		}
		g := Goal{Name: fmt.Sprint(row[0])}
		if len(row) >= 2 {
			g.TargetAmount, _ = parseAmount(row[1])
		}
		if len(row) >= 3 {
			g.Quarter = fmt.Sprint(row[2])
		}
		if len(row) >= 4 {
			g.Status = fmt.Sprint(row[3])
		}
		if len(row) >= 5 {
			g.Description = fmt.Sprint(row[4])
		}
		goals = append(goals, g)
	}
	return goals, nil
}

// AddGoal appends a new goal row to the user's "{userID}_goals" sheet,
// creating the sheet with headers if it doesn't exist yet.
func (c *Client) AddGoal(userID int64, g Goal) error {
	sheetName := strconv.FormatInt(userID, 10) + "_goals"
	headers := []interface{}{"Название", "Целевая сумма (с.)", "Квартал", "Статус", "Описание"}
	if err := c.ensureSheet(sheetName, headers); err != nil {
		return fmt.Errorf("ensure goals sheet: %w", err)
	}
	row := []interface{}{g.Name, g.TargetAmount, g.Quarter, "Активна", g.Description}
	_, err := c.svc.Spreadsheets.Values.
		Append(c.spreadsheetID, sheetName+"!A1", &sheets.ValueRange{Values: [][]interface{}{row}}).
		ValueInputOption("USER_ENTERED").
		Do()
	return err
}

// GetUsers returns all registered user IDs from the "Пользователи" sheet.
func (c *Client) GetUsers() ([]int64, error) {
	resp, err := c.svc.Spreadsheets.Values.
		Get(c.spreadsheetID, usersSheet+"!A2:A10000").
		Do()
	if err != nil {
		return nil, fmt.Errorf("read users sheet: %w", err)
	}
	var ids []int64
	for _, row := range resp.Values {
		if len(row) == 0 {
			continue
		}
		id, err := strconv.ParseInt(fmt.Sprint(row[0]), 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ── private helpers ───────────────────────────────────────────────────────────

// ensureSheet creates the named sheet with the given header row if it doesn't exist.
func (c *Client) ensureSheet(name string, headers []interface{}) error {
	resp, err := c.svc.Spreadsheets.Get(c.spreadsheetID).Do()
	if err != nil {
		return fmt.Errorf("get spreadsheet: %w", err)
	}
	for _, s := range resp.Sheets {
		if s.Properties.Title == name {
			return nil
		}
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{Title: name},
			}},
		},
	}
	if _, err := c.svc.Spreadsheets.BatchUpdate(c.spreadsheetID, req).Do(); err != nil {
		return fmt.Errorf("add sheet %q: %w", name, err)
	}

	_, err = c.svc.Spreadsheets.Values.
		Append(c.spreadsheetID, name+"!A1", &sheets.ValueRange{Values: [][]interface{}{headers}}).
		ValueInputOption("RAW").
		Do()
	return err
}

func parseAmount(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case string:
		return strconv.ParseFloat(strings.ReplaceAll(val, ",", "."), 64)
	default:
		return 0, fmt.Errorf("unexpected type %T", v)
	}
}
