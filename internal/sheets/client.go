// Package sheets reads the legacy Google Sheets data. It is read-only and used only by
// cmd/migrate (constitution v2.0.0, principle I).
//
// Legacy layout (one spreadsheet):
//
//	Пользователи       UserID | Имя | Username | Дата регистрации
//	<id>               Дата | Время | Категория | Сумма | Описание | Месяц   (expense < 0, income > 0)
//	<id>_savings       Дата | Время | Сумма | Описание | Месяц               (deposit > 0, withdrawal < 0)
//	<id>_credit        Дата | Время | Категория | Сумма | Описание | Месяц   (purchase > 0, repayment < 0)
//	<id>_goals         Название | Целевая сумма | Квартал | Статус | Описание
package sheets

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const usersSheet = "Пользователи"

var userSheetRe = regexp.MustCompile(`^\d+$`)

// Client reads the legacy spreadsheet.
type Client struct {
	svc           *sheets.Service
	spreadsheetID string
}

// New creates a Client from a service-account credentials file.
func New(credentialsFile, spreadsheetID string) (*Client, error) {
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials file: %w", err)
	}
	return NewFromJSON(data, spreadsheetID)
}

// NewFromJSON creates a read-only Client from raw service-account JSON.
func NewFromJSON(credentialsJSON []byte, spreadsheetID string) (*Client, error) {
	svc, err := sheets.NewService(context.Background(),
		option.WithAuthCredentialsJSON(option.ServiceAccount, credentialsJSON),
		option.WithScopes(sheets.SpreadsheetsReadonlyScope))
	if err != nil {
		return nil, fmt.Errorf("create sheets service: %w", err)
	}
	return &Client{svc: svc, spreadsheetID: spreadsheetID}, nil
}

// Row is one data row of a ledger sheet. Index is the 1-based sheet row number.
type Row struct {
	Index    int
	Date     string
	Time     string
	Category string
	Amount   string
	Note     string
	Month    string
}

// GoalRow is one row of a <id>_goals sheet.
type GoalRow struct {
	Index   int
	Name    string
	Target  string
	Quarter string
	Status  string
	Note    string
}

// UserRow is one row of the Пользователи sheet.
type UserRow struct {
	ID           int64
	FirstName    string
	Username     string
	RegisteredAt string
}

func MainSheet(id int64) string    { return strconv.FormatInt(id, 10) }
func SavingsSheet(id int64) string { return MainSheet(id) + "_savings" }
func CreditSheet(id int64) string  { return MainSheet(id) + "_credit" }
func GoalsSheet(id int64) string   { return MainSheet(id) + "_goals" }

// UserIDs returns every user known from the registry or from a numeric sheet title.
func (c *Client) UserIDs() ([]int64, map[int64]UserRow, error) {
	meta, err := c.svc.Spreadsheets.Get(c.spreadsheetID).Fields("sheets.properties.title").Do()
	if err != nil {
		return nil, nil, fmt.Errorf("get spreadsheet: %w", err)
	}
	seen := map[int64]bool{}
	var ids []int64
	add := func(id int64) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	users := map[int64]UserRow{}
	rows, err := c.read(usersSheet + "!A2:D")
	if err != nil {
		return nil, nil, err
	}
	for _, r := range rows {
		id, err := strconv.ParseInt(cell(r, 0), 10, 64)
		if err != nil {
			continue
		}
		users[id] = UserRow{ID: id, FirstName: cell(r, 1), Username: cell(r, 2), RegisteredAt: cell(r, 3)}
		add(id)
	}
	for _, s := range meta.Sheets {
		if t := s.Properties.Title; userSheetRe.MatchString(t) {
			if id, err := strconv.ParseInt(t, 10, 64); err == nil {
				add(id)
			}
		}
	}
	return ids, users, nil
}

// ReadMain reads the main sheet (also used for <id>_credit, which has the same columns).
func (c *Client) ReadMain(sheet string) ([]Row, error) {
	rows, err := c.read(sheet + "!A2:F")
	if err != nil {
		return nil, err
	}
	out := make([]Row, 0, len(rows))
	for i, r := range rows {
		out = append(out, Row{Index: i + 2, Date: cell(r, 0), Time: cell(r, 1), Category: cell(r, 2),
			Amount: cell(r, 3), Note: cell(r, 4), Month: cell(r, 5)})
	}
	return out, nil
}

// ReadSavings reads a <id>_savings sheet.
func (c *Client) ReadSavings(sheet string) ([]Row, error) {
	rows, err := c.read(sheet + "!A2:E")
	if err != nil {
		return nil, err
	}
	out := make([]Row, 0, len(rows))
	for i, r := range rows {
		out = append(out, Row{Index: i + 2, Date: cell(r, 0), Time: cell(r, 1),
			Amount: cell(r, 2), Note: cell(r, 3), Month: cell(r, 4)})
	}
	return out, nil
}

// ReadGoals reads a <id>_goals sheet.
func (c *Client) ReadGoals(sheet string) ([]GoalRow, error) {
	rows, err := c.read(sheet + "!A2:E")
	if err != nil {
		return nil, err
	}
	out := make([]GoalRow, 0, len(rows))
	for i, r := range rows {
		out = append(out, GoalRow{Index: i + 2, Name: cell(r, 0), Target: cell(r, 1),
			Quarter: cell(r, 2), Status: cell(r, 3), Note: cell(r, 4)})
	}
	return out, nil
}

// read returns raw rows; numbers come unformatted, dates as displayed strings.
// A missing sheet is not an error: it returns nil.
func (c *Client) read(rng string) ([][]any, error) {
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).
		ValueRenderOption("UNFORMATTED_VALUE").
		DateTimeRenderOption("FORMATTED_STRING").
		Do()
	if err != nil {
		if isMissingSheet(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", rng, err)
	}
	return resp.Values, nil
}

func cell(r []any, i int) string {
	if i >= len(r) || r[i] == nil {
		return ""
	}
	switch v := r[i].(type) {
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}
