// Package catalog holds the pure rules for per-user categories (feature 006): identities,
// name uniqueness, the 50-entry cap, the "at least one visible" rule, ordering and the
// one-time conversion from legacy category names. No I/O.
package catalog

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"

	"expense-bot/internal/category"
)

// IDPrefix marks a category ID; record values without it are legacy names.
const IDPrefix = "c_"

// DefaultID is the ID of a default category key ("food" → "c_food").
func DefaultID(key string) string { return category.DefaultID(key) }

// LegacyID is the deterministic ID for a legacy name found in old records.
func LegacyID(name string) string {
	sum := sha1.Sum([]byte(name))
	return IDPrefix + "l" + hex.EncodeToString(sum[:])[:10]
}

// ClientID derives a category ID from the client-generated UUID (idempotent creates). It hashes
// the whole UUID so that IDs stay distinct even when UUIDs share a prefix.
func ClientID(uuid string) string {
	sum := sha1.Sum([]byte(strings.ToLower(uuid)))
	return IDPrefix + hex.EncodeToString(sum[:])[:12]
}

// IsID reports whether v is a category ID rather than a legacy name.
func IsID(v string) bool { return strings.HasPrefix(v, IDPrefix) }

// NormalizeKey is the uniqueness key of a name: trimmed, lower-case, single spaces.
func NormalizeKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}
