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

// Expense is the data written to a single row in Google Sheets.
type Expense struct {
	Category    string
	Amount      float64
	Description string
}

// MonthStats holds aggregated spending per category for a month.
type MonthStats struct {
	Totals map[string]float64
	Total  float64
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
	creds, err := google.CredentialsFromJSON(
		context.Background(),
		credentialsJSON,
		sheets.SpreadsheetsScope,
	)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	svc, err := sheets.NewService(context.Background(), option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("create sheets service: %w", err)
	}

	return &Client{svc: svc, spreadsheetID: spreadsheetID}, nil
}

// AppendExpense writes one expense row to the current month's sheet.
// Creates the sheet (with headers) if it does not yet exist.
// Returns the sheet name written to.
func (c *Client) AppendExpense(e Expense) (string, error) {
	now := time.Now()
	sheetName := monthSheetName(now)

	if err := c.ensureSheetExists(sheetName); err != nil {
		return "", fmt.Errorf("ensure sheet: %w", err)
	}

	row := []interface{}{
		now.Format("02.01.2006"),
		now.Format("15:04"),
		e.Category,
		e.Amount,
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

// GetMonthStats reads all rows for the given month key (e.g. "2026-03")
// and returns per-category totals.
func (c *Client) GetMonthStats(monthKey string) (*MonthStats, error) {
	sheetName, err := c.findSheetByPrefix(monthKey)
	if err != nil {
		return nil, err
	}
	if sheetName == "" {
		return &MonthStats{Totals: map[string]float64{}}, nil
	}

	resp, err := c.svc.Spreadsheets.Values.
		Get(c.spreadsheetID, sheetName+"!A2:F1000").
		Do()
	if err != nil {
		return nil, fmt.Errorf("get values: %w", err)
	}

	stats := &MonthStats{Totals: map[string]float64{}}
	for _, row := range resp.Values {
		if len(row) < 4 {
			continue
		}
		cat, _ := row[2].(string)
		amt, err := parseAmount(row[3])
		if err != nil || cat == "" {
			continue
		}
		stats.Totals[cat] += amt
		stats.Total += amt
	}
	return stats, nil
}

// ── private helpers ───────────────────────────────────────────────────────────

func (c *Client) ensureSheetExists(name string) error {
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
		return fmt.Errorf("add sheet: %w", err)
	}

	headers := []interface{}{"Дата", "Время", "Категория", "Сумма", "Описание", "Месяц"}
	_, err = c.svc.Spreadsheets.Values.
		Append(c.spreadsheetID, name+"!A1", &sheets.ValueRange{Values: [][]interface{}{headers}}).
		ValueInputOption("RAW").
		Do()
	return err
}

func (c *Client) findSheetByPrefix(prefix string) (string, error) {
	resp, err := c.svc.Spreadsheets.Get(c.spreadsheetID).Do()
	if err != nil {
		return "", fmt.Errorf("get spreadsheet: %w", err)
	}
	for _, s := range resp.Sheets {
		if strings.HasPrefix(s.Properties.Title, prefix) {
			return s.Properties.Title, nil
		}
	}
	return "", nil
}

func monthSheetName(t time.Time) string {
	ru := map[time.Month]string{
		time.January: "Январь", time.February: "Февраль", time.March: "Март",
		time.April: "Апрель", time.May: "Май", time.June: "Июнь",
		time.July: "Июль", time.August: "Август", time.September: "Сентябрь",
		time.October: "Октябрь", time.November: "Ноябрь", time.December: "Декабрь",
	}
	return fmt.Sprintf("%s %s", t.Format("2006-01"), ru[t.Month()])
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
