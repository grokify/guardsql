package guardsql

import "strings"

// Schema describes the entities and fields a user is allowed to query.
type Schema struct {
	Entities map[string]Entity
	MaxLimit int
}

// Entity describes a queryable object type.
type Entity struct {
	Name   string
	Fields map[string]Field
}

// Field describes a queryable/projectable field.
type Field struct {
	Name       string
	Type       FieldType
	Selectable bool
	Filterable bool
	Sortable   bool
}

// FieldType is a coarse type used by validators and backends.
type FieldType string

const (
	FieldString FieldType = "string"
	FieldNumber FieldType = "number"
	FieldBool   FieldType = "bool"
	FieldTime   FieldType = "time"
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

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
