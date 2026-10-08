// Package tgauth verifies Telegram Mini App launch data (initData).
// https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app
package tgauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalid = errors.New("tgauth: invalid init data")
	ErrExpired = errors.New("tgauth: init data expired")
)

// User is the Telegram user that opened the Mini App.
type User struct {
	ID        int64
	FirstName string
	Username  string
}

// Verify checks the initData signature against botToken and its age against maxAge.
func Verify(initData, botToken string, maxAge time.Duration, now time.Time) (User, error) {
	vals, err := url.ParseQuery(initData)
	if err != nil {
		return User{}, ErrInvalid
	}
	hash := vals.Get("hash")
	if hash == "" {
		return User{}, ErrInvalid
	}
	vals.Del("hash")

	if !hmac.Equal([]byte(sign(vals, botToken)), []byte(hash)) {
		return User{}, ErrInvalid
	}

	authDate, err := strconv.ParseInt(vals.Get("auth_date"), 10, 64)
	if err != nil {
		return User{}, ErrInvalid
	}
	if now.Sub(time.Unix(authDate, 0)) > maxAge {
		return User{}, ErrExpired
	}

	var u struct {
		ID        int64  `json:"id"`
		FirstName string `json:"first_name"`
		Username  string `json:"username"`
	}
	if err := json.Unmarshal([]byte(vals.Get("user")), &u); err != nil || u.ID == 0 {
		return User{}, ErrInvalid
	}
	return User{ID: u.ID, FirstName: u.FirstName, Username: u.Username}, nil
}

// sign computes the hex HMAC of the data-check-string (sorted key=value lines, hash excluded).
func sign(vals url.Values, botToken string) string {
	pairs := make([]string, 0, len(vals))
	for k, v := range vals {
		pairs = append(pairs, k+"="+v[0])
	}
	sort.Strings(pairs)

	secret := hmacSHA256([]byte("WebAppData"), []byte(botToken))
	return hex.EncodeToString(hmacSHA256(secret, []byte(strings.Join(pairs, "\n"))))
}

func hmacSHA256(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)
}
