package guardsql

import (
	"fmt"
	"strings"
)

// Schema describes the entities and fields a user is allowed to query.
type Schema struct {
	Entities map[string]Entity `json:"entities"`
	MaxLimit int               `json:"maxLimit,omitempty"`
}

// Entity describes a queryable object type.
type Entity struct {
	Name   string           `json:"name"`
	Fields map[string]Field `json:"fields"`
}

// Field describes a queryable/projectable field.
type Field struct {
	Name       string    `json:"name"`
	Type       FieldType `json:"type"`
	Selectable bool      `json:"selectable"`
	Filterable bool      `json:"filterable"`
	Sortable   bool      `json:"sortable"`
	// Writable governs whether the field may be a target of an INSERT or UPDATE
	// assignment. Like the read capabilities, it is deny-by-default: a field is
	// writable only when explicitly marked, so mutation targets form an explicit
	// allowlist. This is the mass-assignment defense — a caller cannot write a
	// column (for example role or is_admin) unless the schema permits it.
	Writable bool `json:"writable,omitempty"`
	// Class is the field's data classification. Secret fields are never
	// queryable by untrusted callers, internal fields are queryable only when
	// a Config sets AllowInternal, and PII fields only when it sets AllowPII.
	// The zero value (ClassPublic) is ordinary, queryable data. A field has a
	// single Class; when several sensitivities apply, choose the most
	// restrictive: secret, then internal, then PII.
	Class DataClass `json:"class,omitempty"`
}

// DataClass classifies the sensitivity of a field.
type DataClass string

// Data classifications.
const (
	// ClassPublic is ordinary, queryable data (the zero value).
	ClassPublic DataClass = ""
	// ClassPII is personal data, queryable only when Config.AllowPII is set.
	ClassPII DataClass = "pii"
	// ClassInternal is platform-only data that customer-facing queries must
	// not see. It is queryable only when Config.AllowInternal is set.
	ClassInternal DataClass = "internal"
	// ClassSecret is a credential/secret column that is never queryable.
	ClassSecret DataClass = "secret" //nolint:gosec // G101: classification label, not a credential
)

// FieldType is a coarse type used by validators and backends.
type FieldType string

const (
	FieldString FieldType = "string"
	FieldNumber FieldType = "number"
	FieldBool   FieldType = "bool"
	FieldTime   FieldType = "time"
	// FieldJSON is a JSON/JSONB document column. A JSON field may be addressed
	// with a dotted path (e.g. payload.finding_info.title); the compiler emits
	// dialect-specific JSON extraction for the path. Any path under a
	// selectable JSON field is permitted.
	FieldJSON FieldType = "json"
)

// Normalize returns a copy with lower-case entity and field map keys.
func (s Schema) Normalize() Schema {
	out := Schema{Entities: map[string]Entity{}, MaxLimit: s.MaxLimit}
	for name, ent := range s.Entities {
		entName := strings.ToLower(firstNonEmpty(ent.Name, name))
		fields := map[string]Field{}
		for fieldName, field := range ent.Fields {
			field.Name = firstNonEmpty(field.Name, fieldName)
			fields[strings.ToLower(field.Name)] = field
		}
		ent.Name = entName
		ent.Fields = fields
		out.Entities[entName] = ent
	}
	return out
}

// ValidateIdentifiers reports an error if any entity or field name is not a
// safe unquoted SQL identifier (see IsSafeIdentifier). Because identifiers are
// emitted into SQL text rather than bound as parameters, enforcing this over
// the whole schema removes identifier-based injection as a possibility.
func (s Schema) ValidateIdentifiers() error {
	for entName, ent := range s.Entities {
		name := firstNonEmpty(ent.Name, entName)
		if !IsSafeIdentifier(name) {
			return fmt.Errorf("guardsql: entity name %q is not a safe unquoted identifier", name)
		}
		for fieldName, field := range ent.Fields {
			fn := firstNonEmpty(field.Name, fieldName)
			if !IsSafeIdentifier(fn) {
				return fmt.Errorf("guardsql: field name %q on entity %q is not a safe unquoted identifier", fn, name)
			}
		}
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
