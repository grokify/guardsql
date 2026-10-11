package guardsql

import "strings"

// FieldRef is a field name resolved against a schema entity. For a plain column
// JSONPath is empty; for a dotted reference into a FieldJSON column (e.g.
// payload.finding_info.title) Name is the base column ("payload") and JSONPath
// is the extraction path (["finding_info","title"]).
type FieldRef struct {
	Field    Field
	Name     string
	JSONPath []string
}

// IsJSONPath reports whether the reference extracts a path from a JSON column.
func (r FieldRef) IsJSONPath() bool { return len(r.JSONPath) > 0 }

// Key returns the canonical identity of the reference, used to compare
// SELECT/GROUP BY/ORDER BY references. Two references are the same field iff
// their keys match, so distinct JSON paths into the same column are distinct.
func (r FieldRef) Key() string {
	if len(r.JSONPath) == 0 {
		return r.Name
	}
	return r.Name + "." + strings.Join(r.JSONPath, ".")
}

// resolveField resolves a possibly alias-qualified, possibly dotted field name
// against an entity. Recognized forms:
//
//	col                 -> plain column
//	alias.col           -> alias stripped, plain column
//	jsoncol.a.b         -> JSON column "jsoncol" with path ["a","b"]
//	alias.jsoncol.a.b   -> alias stripped, JSON column + path
//
// A dotted reference is only accepted when its base column is a FieldJSON, and
// every path segment is a safe identifier (letters, digits, underscore).
func resolveField(ent Entity, name string, aliases map[string]string) (FieldRef, bool) {
	raw := strings.ToLower(strings.TrimSpace(name))

	// Strip a leading alias segment if it matches a known alias.
	if idx := strings.Index(raw, "."); idx >= 0 && aliases != nil {
		if _, ok := aliases[raw[:idx]]; ok {
			raw = raw[idx+1:]
		}
	}

	// Exact match: plain column.
	if f, ok := ent.Fields[raw]; ok {
		return FieldRef{Field: f, Name: raw}, true
	}

	// Dotted: base JSON column + extraction path.
	if idx := strings.Index(raw, "."); idx >= 0 {
		base := raw[:idx]
		if f, ok := ent.Fields[base]; ok && f.Type == FieldJSON {
			segs := strings.Split(raw[idx+1:], ".")
			for _, seg := range segs {
				if !isSafePathSegment(seg) {
					return FieldRef{}, false
				}
			}
			return FieldRef{Field: f, Name: base, JSONPath: segs}, true
		}
	}

	return FieldRef{}, false
}

// isSafePathSegment reports whether a JSON path segment is a bare ASCII
// identifier (letters, digits, underscore), matching the identifier lockdown.
// This is defense in depth: segments already come from lexed identifiers, but
// the compiler re-checks before emitting a path so no separator, quote, or
// non-ASCII homoglyph can ever reach generated SQL.
func isSafePathSegment(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}
