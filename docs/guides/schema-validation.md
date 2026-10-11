# Schema Validation

GuardSQL schemas define what a query may reference. They are an allowlist, not
a database introspection cache.

## Schema Shape

```go
schema := guardsql.Schema{
    Entities: map[string]guardsql.Entity{
        "roadmap_items": {
            Fields: map[string]guardsql.Field{
                "id":     {Type: guardsql.FieldString, Selectable: true, Filterable: true, Sortable: true},
                "name":   {Type: guardsql.FieldString, Selectable: true, Filterable: true, Sortable: true},
                "score":  {Type: guardsql.FieldNumber, Selectable: true, Filterable: true, Sortable: true},
                "status": {Type: guardsql.FieldString, Selectable: true, Filterable: true, Sortable: true},
            },
        },
    },
}
```

## Field Types

Supported coarse field types:

| Type | Use |
|------|-----|
| `string` | identifiers, labels, enum-like values |
| `number` | integer and floating-point measures |
| `bool` | boolean flags |
| `time` | dates and timestamps |
| `json` | JSON/JSONB documents, addressable by dotted path (see below) |

Backends may maintain richer internal types. GuardSQL field types are intended
for query validation and UI-safe metadata.

## Capabilities

Each capability is an independent, deny-by-default allowlist — a field has a
capability only when it is set. This is where per-column access is configured.

| Capability | Grants |
|------------|--------|
| `Selectable` | appear in `SELECT` and aggregates |
| `Filterable` | appear in `WHERE` |
| `Sortable` | appear in `ORDER BY` and `GROUP BY` |
| `Writable` | be an assignment target of an INSERT/UPDATE (see [Mutations](mutations.md)) |

`Writable` is the mass-assignment defense: because it is deny-by-default and
independent of the read capabilities, a caller cannot write a column such as
`role` or `is_admin` unless the schema explicitly permits it.

```go
"status": {Type: guardsql.FieldString, Selectable: true, Filterable: true, Sortable: true, Writable: true},
"id":     {Type: guardsql.FieldNumber, Selectable: true, Filterable: true, Sortable: true}, // read-only
```

## Data Classification

Each field carries a `Class` that controls visibility per trust level:

| Class | Meaning | Queryable when |
|-------|---------|----------------|
| `ClassPublic` (default) | ordinary / audit data | always |
| `ClassPII` | personal data | `Config.AllowPII` is set |
| `ClassInternal` | platform-only data not meant for customers | `Config.AllowInternal` is set |
| `ClassSecret` | credentials / secrets | `Config.AllowSecret` is set |

A field has a single `Class`. When several sensitivities apply, use the most
restrictive: secret, then internal, then PII, then public. A field that is both
internal and PII is `ClassInternal`, so `AllowPII` alone does not expose it.

Denied fields are stripped from the effective schema, so a query that references
one fails as an unknown field without disclosing that the column exists. See
[Config & Trust Modes](trust-modes.md#data-classification).

### Classification from a data catalog

`Class` can always be set by hand, but the recommended source of classification
is a [`datacatalog`](https://github.com/grokify/datacatalog) catalog, which
records visibility, PII, and secret tags explicitly for every field.
`FromCatalog` maps a validated catalog to a `Schema` so that exactly the fields
`datacatalog` would allow in a user query survive the untrusted profile. See
[Schema from a Data Catalog](catalog-schema.md).

## Normalization

`Schema.Normalize` lowercases entity and field map keys and fills missing field
names from map keys. Query validation compares names case-insensitively.

## Dotted Fields and JSON Paths

A dotted name whose base column is a `json` field is a JSON extraction path. The
[compiler](compiler.md) emits dialect-specific JSON extraction for it, and every
path segment is re-validated as a bare identifier:

```sql
SELECT payload.finding_info.title FROM events
```

Qualified source aliases are also supported. Validation only strips known source
aliases, so a real dotted field such as `custom.launch_tier` remains addressable.

## Semantic Analysis

`Analyze` uses the same schema allowlist to produce resolved query requirements:

```go
analysis, err := guardsql.Analyze(q, schema)
if err != nil {
    return err
}
requirements := analysis.Requirements()
```

The requirement set includes:

- query operations
- resolved entities
- field usages such as `select`, `filter`, `sort`, `group`, `aggregate`,
  `join`, and `having`
- aggregate functions
- joins

This lets host applications authorize exact query requirements with systems such
as SystemForge/SpiceDB before compiling or executing a backend query.
