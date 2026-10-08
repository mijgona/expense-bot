package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime settings for the bot.
type Config struct {
	BotToken       string  `json:"bot_token"`
	SpreadsheetID  string  `json:"spreadsheet_id"`
	Salary         int     `json:"salary"`
	AllowedUserIDs []int64 `json:"allowed_user_ids"`
	GeminiAPIKey   string  `json:"gemini_api_key"`

	// WebAppURL is the Mini App URL (Render static site). Empty → bot replies without the app button.
	WebAppURL string `json:"webapp_url"`
	// FirestoreProjectID overrides project_id from the service-account credentials.
	FirestoreProjectID string `json:"firestore_project_id"`
	// DevUserID skips initData verification and acts as this user. Local development only.
	DevUserID int64 `json:"dev_user_id"`
	// PayrollFakeToday ("2006-01-02T15:04", Dushanbe) freezes the payday reminder clock. Local only.
	PayrollFakeToday string `json:"payroll_fake_today"`
}

// Load reads config from a JSON file.
// Environment variables always override file values:
//
//	BOT_TOKEN, SPREADSHEET_ID, SALARY, GEMINI_API_KEY, ALLOWED_USER_IDS,
//	WEBAPP_URL, FIRESTORE_PROJECT_ID, DEV_USER_ID
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyEnvOverrides()

	return &cfg, cfg.validate()
}

// LoadFromEnv builds Config entirely from environment variables.
// Required: BOT_TOKEN.
// Optional: SALARY (default 16000), SPREADSHEET_ID (migration only), WEBAPP_URL,
// FIRESTORE_PROJECT_ID, GEMINI_API_KEY, ALLOWED_USER_IDS, DEV_USER_ID (refused on Render).
func LoadFromEnv() (*Config, error) {
	cfg := &Config{Salary: 16000}
	cfg.applyEnvOverrides()
	return cfg, cfg.validate()
}

// applyEnvOverrides overwrites any non-empty env var value over the current field.
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("BOT_TOKEN"); v != "" {
		c.BotToken = v
	}
	if v := os.Getenv("SPREADSHEET_ID"); v != "" {
		c.SpreadsheetID = v
	}
	if v := os.Getenv("SALARY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Salary = n
		}
	}
	if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		c.GeminiAPIKey = v
	}
	if v := os.Getenv("WEBAPP_URL"); v != "" {
		c.WebAppURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("FIRESTORE_PROJECT_ID"); v != "" {
		c.FirestoreProjectID = v
	}
	if v := os.Getenv("DEV_USER_ID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.DevUserID = id
		}
	}
	if v := os.Getenv("PAYROLL_FAKE_TODAY"); v != "" {
		c.PayrollFakeToday = v
	}
	if v := os.Getenv("ALLOWED_USER_IDS"); v != "" {
		c.AllowedUserIDs = nil
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if id, err := strconv.ParseInt(s, 10, 64); err == nil {
				c.AllowedUserIDs = append(c.AllowedUserIDs, id)
			}
		}
	}
}

func (c *Config) validate() error {
	if c.BotToken == "" {
		return fmt.Errorf("BOT_TOKEN / bot_token is required")
	}
	if c.Salary <= 0 {
		return fmt.Errorf("SALARY / salary must be > 0")
	}
	if c.DevUserID != 0 && os.Getenv("RENDER") != "" {
		return fmt.Errorf("DEV_USER_ID must not be set on Render: it disables initData verification")
	}
	if c.PayrollFakeToday != "" && os.Getenv("RENDER") != "" {
		return fmt.Errorf("PAYROLL_FAKE_TODAY must not be set on Render")
	}
	c.WebAppURL = strings.TrimRight(c.WebAppURL, "/")
	return nil
}
