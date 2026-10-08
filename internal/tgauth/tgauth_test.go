package tgauth

import (
	"errors"
	"net/url"
	"strconv"
	"testing"
	"time"
)

const token = "123456:TEST-token"

var now = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

// build returns signed initData the way Telegram does.
func build(fields map[string]string, tok string) string {
	vals := url.Values{}
	for k, v := range fields {
		vals.Set(k, v)
	}
	vals.Set("hash", sign(vals, tok))
	return vals.Encode()
}

func fields(age time.Duration, user string) map[string]string {
	return map[string]string{
		"auth_date": strconv.FormatInt(now.Add(-age).Unix(), 10),
		"query_id":  "AAH",
		"user":      user,
	}
}

const alice = `{"id":42,"first_name":"Алиса","username":"alice","language_code":"ru"}`

func TestValid(t *testing.T) {
	u, err := Verify(build(fields(time.Minute, alice), token), token, 24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 42 || u.FirstName != "Алиса" || u.Username != "alice" {
		t.Errorf("user = %+v", u)
	}
}

func TestNoUsername(t *testing.T) {
	u, err := Verify(build(fields(time.Minute, `{"id":7,"first_name":"Bob"}`), token), token, 24*time.Hour, now)
	if err != nil || u.Username != "" {
		t.Errorf("u=%+v err=%v", u, err)
	}
}

func TestTamperedUser(t *testing.T) {
	vals, _ := url.ParseQuery(build(fields(time.Minute, alice), token))
	vals.Set("user", `{"id":43,"first_name":"Mallory"}`)
	if _, err := Verify(vals.Encode(), token, 24*time.Hour, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestWrongToken(t *testing.T) {
	data := build(fields(time.Minute, alice), "999:OTHER")
	if _, err := Verify(data, token, 24*time.Hour, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestMissingHash(t *testing.T) {
	vals := url.Values{}
	for k, v := range fields(time.Minute, alice) {
		vals.Set(k, v)
	}
	if _, err := Verify(vals.Encode(), token, 24*time.Hour, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
	if _, err := Verify("", token, 24*time.Hour, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty: err = %v, want ErrInvalid", err)
	}
}

func TestExpiry(t *testing.T) {
	if _, err := Verify(build(fields(25*time.Hour, alice), token), token, 24*time.Hour, now); !errors.Is(err, ErrExpired) {
		t.Errorf("25h: err = %v, want ErrExpired", err)
	}
	if _, err := Verify(build(fields(23*time.Hour, alice), token), token, 24*time.Hour, now); err != nil {
		t.Errorf("23h: err = %v, want nil", err)
	}
}

func TestMissingUser(t *testing.T) {
	f := fields(time.Minute, alice)
	delete(f, "user")
	if _, err := Verify(build(f, token), token, 24*time.Hour, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}
