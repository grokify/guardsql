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

Backends may maintain richer internal types. GuardSQL field types are intended
for query validation and UI-safe metadata.

## Capabilities

Fields can independently be:

- selectable
- filterable
- sortable

Grouping uses the sortable capability in current policy checks.

## Normalization

`Schema.Normalize` lowercases entity and field map keys and fills missing field
names from map keys. Query validation compares names case-insensitively.

## Dotted Fields

Dotted field names are supported for custom or semantic fields, for example:

```sql
SELECT custom.launch_tier FROM roadmap_items
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
