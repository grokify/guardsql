package guardsql

import (
	"fmt"
	"strings"

	"github.com/grokify/datacatalog"
)

// CatalogOption customizes FromCatalog.
type CatalogOption func(*catalogOptions)

type catalogOptions struct {
	writable      map[fieldKey]bool
	nonFilterable map[fieldKey]bool
}

type fieldKey struct{ entity, field string }

// WithWritable marks the named fields of an entity as Writable, making them
// valid INSERT/UPDATE assignment targets. Fields are read-only by default.
// FromCatalog returns an error if the entity or a field is not in the catalog.
func WithWritable(entity string, fields ...string) CatalogOption {
	return func(o *catalogOptions) {
		for _, f := range fields {
			o.writable[fieldKey{entity, f}] = true
		}
	}
}

// WithNonFilterable marks the named fields of an entity as not Filterable, so
// they cannot appear in WHERE. FromCatalog returns an error if the entity or a
// field is not in the catalog.
func WithNonFilterable(entity string, fields ...string) CatalogOption {
	return func(o *catalogOptions) {
		for _, f := range fields {
			o.nonFilterable[fieldKey{entity, f}] = true
		}
	}
}

// FromCatalog builds a Schema from a datacatalog Catalog, mapping the
// catalog's classification onto Field.Class so that Config.effectiveSchema
// blocks exactly what datacatalog's AllowedInQuery blocks.
//
// The catalog is validated first and its validation error is returned
// unchanged, so an unclassified field can never enter a schema. Every field is
// Selectable, Filterable, and Sortable and not Writable unless an option says
// otherwise (see WithWritable and WithNonFilterable).
//
// A guardsql field has a single Class, so when several datacatalog attributes
// apply the most restrictive for query purposes wins: secret, then internal
// visibility, then PII, then public. Use AllowSecret, AllowInternal, and
// AllowPII on Config to admit each class (NewUntrustedConfig admits none,
// NewTrustedConfig admits all).
//
// Entity and field names are lower-cased as by Schema.Normalize; two names
// that differ only in case are an error.
func FromCatalog(c *datacatalog.Catalog, opts ...CatalogOption) (Schema, error) {
	if c == nil {
		return Schema{}, fmt.Errorf("guardsql: nil catalog")
	}
	if err := c.Validate(); err != nil {
		return Schema{}, err
	}
	o := catalogOptions{writable: map[fieldKey]bool{}, nonFilterable: map[fieldKey]bool{}}
	for _, opt := range opts {
		opt(&o)
	}
	if err := checkCatalogOptions(c, o); err != nil {
		return Schema{}, err
	}

	out := Schema{Entities: make(map[string]Entity, len(c.Entities))}
	for _, e := range c.Entities {
		entKey := strings.ToLower(e.Name)
		if _, dup := out.Entities[entKey]; dup {
			return Schema{}, fmt.Errorf("guardsql: catalog entity %q collides with another entity by case", e.Name)
		}
		fields := make(map[string]Field, len(e.Fields))
		for _, f := range e.Fields {
			fieldKeyLower := strings.ToLower(f.Name)
			if _, dup := fields[fieldKeyLower]; dup {
				return Schema{}, fmt.Errorf("guardsql: catalog field %q on entity %q collides with another field by case", f.Name, e.Name)
			}
			k := fieldKey{e.Name, f.Name}
			fields[fieldKeyLower] = Field{
				Name:       fieldKeyLower,
				Type:       fieldTypeFromCatalog(f.Type),
				Selectable: true,
				Filterable: !o.nonFilterable[k],
				Sortable:   true,
				Writable:   o.writable[k],
				Class:      classFromCatalog(f),
			}
		}
		out.Entities[entKey] = Entity{Name: entKey, Fields: fields}
	}
	return out, nil
}

func checkCatalogOptions(c *datacatalog.Catalog, o catalogOptions) error {
	for _, set := range []map[fieldKey]bool{o.writable, o.nonFilterable} {
		for k := range set {
			if _, ok := c.Field(k.entity, k.field); !ok {
				return fmt.Errorf("guardsql: catalog option names unknown field %q on entity %q", k.field, k.entity)
			}
		}
	}
	return nil
}

// classFromCatalog maps a datacatalog field to the single, most restrictive
// guardsql class: secret, then internal, then PII, then public. Anything that
// is not explicitly public visibility is treated as internal (fail closed).
func classFromCatalog(f datacatalog.Field) DataClass {
	switch {
	case f.Secret:
		return ClassSecret
	case f.Visibility != datacatalog.VisibilityPublic:
		return ClassInternal
	case f.PII:
		return ClassPII
	}
	return ClassPublic
}

// fieldTypeFromCatalog maps the catalog's coarse type to a guardsql FieldType.
// Unset and bytes types map to FieldString: guardsql has no binary type, and
// an opaque string is the most conservative comparison domain.
func fieldTypeFromCatalog(t datacatalog.Type) FieldType {
	switch t {
	case datacatalog.TypeNumber:
		return FieldNumber
	case datacatalog.TypeBool:
		return FieldBool
	case datacatalog.TypeTime:
		return FieldTime
	case datacatalog.TypeJSON:
		return FieldJSON
	default: // string, bytes, unset
		return FieldString
	}
}
