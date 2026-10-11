# Schema from a Data Catalog

GuardSQL decides which columns a query may reference from each field's `Class`.
Rather than tagging every field by hand, build the schema from a
[`datacatalog`](https://github.com/grokify/datacatalog) catalog, the recommended
source of classification. A catalog states, for every field, an explicit
visibility (`public` or `internal`) and independent `pii` and `secret` tags;
there is no default, so an unclassified field cannot slip through.

`Field.Class` is still an ordinary field and can be set by hand for schemas that
do not use a catalog.

## Building the Schema

```go
import (
    "github.com/grokify/datacatalog"
    "github.com/grokify/guardsql"
)

cat := &datacatalog.Catalog{
    Name: "app",
    Entities: []datacatalog.Entity{{
        Name: "users",
        Fields: []datacatalog.Field{
            {Name: "id", Type: datacatalog.TypeNumber, Visibility: datacatalog.VisibilityPublic},
            {Name: "email", Type: datacatalog.TypeString, Visibility: datacatalog.VisibilityPublic, PII: true},
            {Name: "shard", Type: datacatalog.TypeString, Visibility: datacatalog.VisibilityInternal},
            {Name: "api_token", Type: datacatalog.TypeString, Visibility: datacatalog.VisibilityInternal, Secret: true},
        },
    }},
}

schema, err := guardsql.FromCatalog(cat)
if err != nil {
    return err // includes *datacatalog.ValidationError
}
cfg := guardsql.NewUntrustedConfig(schema, guardsql.DialectPostgres)
_, err = cfg.Compile(`SELECT shard FROM users LIMIT 10`) // unknown select field
```

`FromCatalog` validates the catalog first and returns its validation error
unchanged, so an unclassified field or a secret field that is not internal
never produces a schema.

## Classification Mapping

A guardsql field has a single `Class`, so when several catalog attributes apply
the most restrictive one for query purposes wins:

| Catalog field | `Class` | Untrusted | `AllowPII` only | Trusted |
|---------------|---------|-----------|-----------------|---------|
| secret | `ClassSecret` | blocked | blocked | allowed |
| internal (with or without PII) | `ClassInternal` | blocked | blocked | allowed |
| public, PII | `ClassPII` | blocked | allowed | allowed |
| public | `ClassPublic` | allowed | allowed | allowed |

This matches `datacatalog`'s `Field.AllowedInQuery(allowPII)`: with
`AllowPII` false (the untrusted profile) the surviving fields are exactly those
for which `AllowedInQuery(false)` is true, and with `AllowPII` true exactly
those for which `AllowedInQuery(true)` is true. `AllowInternal` and
`AllowSecret` admit the internal and secret classes, and are enabled only by the
trusted profile.

## Types and Capabilities

| Catalog type | `FieldType` |
|--------------|-------------|
| `string`, unset | `string` |
| `number` | `number` |
| `bool` | `bool` |
| `time` | `time` |
| `json` | `json` |
| `bytes` | `string` (guardsql has no binary type) |

Every field is `Selectable`, `Filterable`, and `Sortable`, and not `Writable`.
Adjust by name with options; naming an unknown entity or field is an error:

```go
schema, err := guardsql.FromCatalog(cat,
    guardsql.WithWritable("users", "display_name"),
    guardsql.WithNonFilterable("users", "notes"),
)
```

Entity and field names are lower-cased as by `Schema.Normalize`; two names that
differ only by case are rejected.
