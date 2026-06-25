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
}

// Load reads config from a JSON file.
// Environment variables always override file values:
//
//	BOT_TOKEN, SPREADSHEET_ID, SALARY
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
// Required: BOT_TOKEN, SPREADSHEET_ID.
// Optional: SALARY (default 10000; set a real value via env/config).
func LoadFromEnv() (*Config, error) {
	cfg := &Config{Salary: 10000}
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
	if c.SpreadsheetID == "" {
		return fmt.Errorf("SPREADSHEET_ID / spreadsheet_id is required")
	}
	if c.Salary <= 0 {
		return fmt.Errorf("SALARY / salary must be > 0")
	}
	return nil
}
