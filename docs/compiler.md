# SQL Compiler

GuardSQL includes a built-in compiler that transpiles a validated AST into
parameterized, dialect-specific SQL. This realizes the core security principle:
the executed SQL is **regenerated from the validated AST**, never the original
user text.

```text
query text -> parse -> validate -> policy -> Compile -> parameterized SQL + args
```

Because only constructs the AST can represent — and the emitter knows how to
write — can ever reach the database, the compiler is default-deny by
construction. Anything else (comments, stacked statements, unknown functions,
encoding tricks) is structurally impossible to emit.

## Usage

```go
q, err := guardsql.Parse(input)
if err != nil {
    return err
}
if err := guardsql.Validate(q, schema); err != nil {
    return err
}
out, err := guardsql.Compile(q, guardsql.CompileOptions{
    Dialect:           guardsql.DialectSQLite,
    Schema:            schema,
    StrictIdentifiers: true,
})
if err != nil {
    return err
}
rows, err := db.Query(out.SQL, out.Args...)
```

`CompiledSQL` carries the SQL string and its positional bind arguments:

```go
type CompiledSQL struct {
    SQL  string
    Args []any
}
```

Most callers use the higher-level [Config](trust-modes.md), which runs
parse → validate → policy → compile in one call with secure defaults.

## Guarantees

- **Parameterized values.** Every literal becomes a bind parameter (`?` /
  `$1`), never string-concatenated. Value-level injection is impossible.
- **Resolved identifiers.** Every table and column is re-resolved against the
  schema and re-quoted per dialect. Raw user identifier text is never passed
  through; unknown identifiers are an error.
- **Strict identifiers.** With `StrictIdentifiers` (the Config default), every
  schema identifier must match the unquoted identifier charset. See
  [Trust Modes](trust-modes.md#identifier-safety).

## Dialects

| Dialect | Constant | Identifier quoting | Placeholders |
|---------|----------|--------------------|--------------|
| SQLite | `DialectSQLite` | `"col"` | `?` |
| PostgreSQL | `DialectPostgres` | `"col"` | `$1`, `$2`, … |
| MySQL / Dolt | `DialectMySQL` | `` `col` `` | `?` |

## JSON Extraction

A field typed `FieldJSON` in the schema can be addressed with a dotted path. The
compiler emits dialect-specific JSON extraction; the path segments are
validated as bare identifiers (letters, digits, underscore).

Given `payload` typed as `FieldJSON`:

```sql
SELECT payload.finding_info.title AS t, count(*) AS n
FROM events
GROUP BY payload.finding_info.title
```

compiles to:

=== "SQLite"

    ```sql
    SELECT json_extract("payload", ?) AS "t", COUNT(*) AS "n"
    FROM "events" GROUP BY json_extract("payload", ?)
    -- args: ["$.finding_info.title", "$.finding_info.title"]
    ```

=== "PostgreSQL"

    ```sql
    SELECT jsonb_extract_path_text("payload"::jsonb, $1, $2) AS "t", ...
    -- args: ["finding_info", "title", ...]
    ```

=== "MySQL"

    ```sql
    SELECT JSON_UNQUOTE(JSON_EXTRACT(`payload`, ?)) AS `t`, ...
    -- args: ["$.finding_info.title", ...]
    ```

JSON path segments may be numeric to address array elements (for example
`resources.0.uid`). JSON values are treated as dynamically typed, so static
value-type checks are relaxed for path references.

## Supported Query Shape

The compiler currently supports single-source read queries: `SELECT` (columns,
aliases, and the `COUNT`/`SUM`/`AVG`/`MIN`/`MAX` aggregates), `WHERE`,
`GROUP BY`, `HAVING`, `ORDER BY`, and `LIMIT`. Joins, CTEs, and nested sources
are represented in the AST but are not yet emitted by the built-in compiler;
compile them with a backend-specific compiler if needed.

For trusted mutations (INSERT/UPDATE/DELETE), see [Mutations](mutations.md).
