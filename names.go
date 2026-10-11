package guardsql

import (
	"fmt"
	"strings"
)

// maxIdentifierLen bounds identifier length as a defensive sanity limit.
const maxIdentifierLen = 128

// IsSafeIdentifier reports whether s is a valid unquoted SQL identifier: an
// ASCII letter or underscore, followed by ASCII letters, digits, or
// underscores, up to 128 bytes.
//
// Identifiers (table and column names) cannot be bound as query parameters —
// most drivers only parameterize values — so they must be emitted into SQL
// text. Restricting them to this charset (and rejecting quotes, whitespace,
// and non-ASCII homoglyphs) makes identifier-based SQL injection structurally
// impossible, independent of quoting. Databases whose schema uses only
// unquoted-style identifiers can therefore be locked down completely.
func IsSafeIdentifier(s string) bool {
	if s == "" || len(s) > maxIdentifierLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_':
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// OutputName returns the result column name for a SELECT item.
func OutputName(item SelectItem) string {
	return outputName(item)
}

func outputName(item SelectItem) string {
	if item.Alias != "" {
		return item.Alias
	}
	if item.Aggregate != nil {
		if item.Aggregate.Star {
			return strings.ToLower(string(item.Aggregate.Func)) + "_all"
		}
		return strings.ToLower(fmt.Sprintf("%s_%s", item.Aggregate.Func, item.Aggregate.Field))
	}
	return item.Field
}
