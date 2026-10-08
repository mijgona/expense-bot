// Package telegram is a minimal raw Bot API client and the slimmed-down bot
// (contracts/bot.md). telegram-bot-api v5.5.1 has no web_app buttons, hence raw HTTP.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"
)

const apiBase = "https://api.telegram.org/bot"

// Client calls the Telegram Bot API.
type Client struct {
	token string
	http  *http.Client
}

// NewClient creates a client. The HTTP timeout exceeds the long-poll timeout.
func NewClient(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 40 * time.Second}}
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

func (c *Client) call(ctx context.Context, method string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+c.token+"/"+method, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()
	var r apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("telegram %s: HTTP %d: decode: %w", method, resp.StatusCode, err)
	}
	if !r.OK {
		return fmt.Errorf("telegram %s: HTTP %d: %s", method, resp.StatusCode, r.Description)
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

func (c *Client) callJSON(ctx context.Context, method string, payload, out any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.call(ctx, method, bytes.NewReader(b), "application/json", out)
}

// GetMe returns the bot's username.
func (c *Client) GetMe(ctx context.Context) (string, error) {
	var u User
	if err := c.callJSON(ctx, "getMe", struct{}{}, &u); err != nil {
		return "", err
	}
	return u.Username, nil
}

// GetUpdates long-polls for messages.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout int) ([]Update, error) {
	var ups []Update
	err := c.callJSON(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         timeout,
		"allowed_updates": []string{"message"},
	}, &ups)
	return ups, err
}

func appButton(webAppURL string) map[string]any {
	return map[string]any{"inline_keyboard": [][]map[string]any{{
		{"text": "📊 Открыть приложение", "web_app": map[string]string{"url": webAppURL}},
	}}}
}

// SendMessage sends Markdown text; webAppURL != "" adds the "Открыть приложение" button.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text, webAppURL string) error {
	p := map[string]any{"chat_id": chatID, "text": text, "parse_mode": "Markdown"}
	if webAppURL != "" {
		p["reply_markup"] = appButton(webAppURL)
	}
	return c.callJSON(ctx, "sendMessage", p, nil)
}

// SendDocument uploads data as a file with a caption.
func (c *Client) SendDocument(ctx context.Context, chatID int64, filename string, data []byte, caption, webAppURL string) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	_ = mw.WriteField("caption", caption)
	if webAppURL != "" {
		kb, _ := json.Marshal(appButton(webAppURL))
		_ = mw.WriteField("reply_markup", string(kb))
	}
	fw, err := mw.CreateFormFile("document", filename)
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	return c.call(ctx, "sendDocument", &buf, mw.FormDataContentType(), nil)
}

// SetChatMenuButton sets the default menu button to open the Mini App.
func (c *Client) SetChatMenuButton(ctx context.Context, text, url string) error {
	return c.callJSON(ctx, "setChatMenuButton", map[string]any{
		"menu_button": map[string]any{"type": "web_app", "text": text, "web_app": map[string]string{"url": url}},
	}, nil)
}
