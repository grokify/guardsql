# Config & Trust Modes

`Config` is the recommended high-level entry point. It bundles the schema, the
target dialect, data-classification visibility, and identifier safety, and runs
the full pipeline (parse → validate → policy → compile) with secure defaults.

`Config` is JSON-serializable, so an application can load "what the database is
and how it may be queried" from a file.

## Two Trust Profiles

The same engine serves both third-party (customer-supplied) SQL and internal,
application-authored SQL. Two constructors provide hardened defaults:

```go
// Customer / Metabase-style SQL: read-only, secret + internal + PII columns stripped,
// identifier lockdown, LIMIT required.
cfg := guardsql.NewUntrustedConfig(schema, guardsql.DialectSQLite)
out, err := cfg.Compile(customerQuery)

// Internal application SQL: reads + mutations, full column access.
tcfg := guardsql.NewTrustedConfig(schema, guardsql.DialectPostgres)
```

| | Untrusted | Trusted |
|---|-----------|---------|
| Source | parsed query text | typed builder (reads: text too) |
| Operations | read-only | read + INSERT/UPDATE/DELETE |
| Secret / internal / PII columns | stripped in the AST | accessible |
| Identifiers | locked to safe charset | locked to safe charset |
| Values | bound parameters | bound parameters |
| `LIMIT` | required | — |
| UPDATE/DELETE `WHERE` | n/a | required by default |

Trust removes *access restrictions*, not the *safety* of regenerating
parameterized SQL from the AST: trusted queries are still parameterized,
identifier-locked, and schema-checked.

## One-Step Compile

```go
out, err := cfg.Compile(input) // parse -> check -> compile
// or, to validate without compiling:
q, err := cfg.Check(input)
```

## Data Classification

Each schema `Field` carries a `Class`:

| Class | Meaning | Queryable when |
|-------|---------|----------------|
| `ClassPublic` (default) | ordinary / audit data | always |
| `ClassPII` | personal data | `Config.AllowPII` is set |
| `ClassInternal` | platform-only data not meant for customers | `Config.AllowInternal` is set |
| `ClassSecret` | credentials / secrets | `Config.AllowSecret` is set |

```go
"email":         {Type: guardsql.FieldString, Selectable: true, Class: guardsql.ClassPII},
"password_hash": {Type: guardsql.FieldString, Selectable: true, Class: guardsql.ClassSecret},
```

Denied fields are removed from the effective schema before validation and
compilation, so a query that references one fails as an **unknown field** — it
never discloses that the column exists. This is the automatic AST-step stripping
that separates the untrusted and trusted profiles: `NewUntrustedConfig` allows
none of the PII, internal, or secret columns; `NewTrustedConfig` allows all three.

## Identifier Safety

Table and column names cannot be bound as query parameters — most drivers only
parameterize *values* — so identifiers are emitted into SQL text. GuardSQL's
default defense is a strict charset rather than relying on quoting alone.

`IsSafeIdentifier` accepts an ASCII letter or underscore followed by ASCII
letters, digits, or underscores (up to 128 bytes). It rejects quotes,
whitespace, `;`, `--`, and non-ASCII homoglyphs.

```go
guardsql.IsSafeIdentifier("status_id") // true
guardsql.IsSafeIdentifier(`x"; DROP`)  // false
```

By default (`AllowUnsafeIdentifiers` is false), `Config` and the compiler reject
any schema whose entity or field names are not safe identifiers — *before any
SQL is emitted*. For databases whose columns are all unquoted-style (the common
case), this makes identifier-based injection structurally impossible.
`Schema.ValidateIdentifiers()` performs the check directly.

Set `AllowUnsafeIdentifiers` only for a database that genuinely relies on quoted
identifiers; identifiers are still quoted per dialect in that case.

## Row Scope (Tenant / IDOR Isolation)

GuardSQL cannot *decide* whether a principal may see a given row — that needs the
session identity and ownership data, which live in the application. But it can
*enforce* an app-supplied decision so it cannot be bypassed. `RowScope` returns a
mandatory predicate for each queried entity, and GuardSQL ANDs it in at the top
level of every compiled SELECT, UPDATE, and DELETE. Because it is a top-level
`AND`, no user-supplied `WHERE` — however it uses `OR` — can widen it.

```go
cfg := guardsql.NewUntrustedConfig(schema, guardsql.DialectPostgres)
cfg.RowScope = guardsql.ScopeEquals("tenant_id", sessionTenant) // trusted value
cfg.RequireRowScope = true                                      // fail-closed

out, err := cfg.Compile(`SELECT id FROM events WHERE status = "open" LIMIT 50`)
// SELECT "id" FROM "events" WHERE ("status" = $1 AND "tenant_id" = $2) LIMIT 50
```

Even a deliberate widening attempt stays scoped:

```sql
-- user input:
SELECT id FROM events WHERE tenant_id = 999 OR id > 0 LIMIT 50
-- compiled:
SELECT "id" FROM "events"
WHERE (("tenant_id" = $1 OR "id" > $2) AND "tenant_id" = $3) LIMIT 50
-- $3 is the session tenant; the mandatory term always holds.
```

Behavior:

- **Fail-closed on missing column.** If a queried entity has no scope column,
  compilation fails rather than returning unscoped data. `ScopeEquals` applies to
  every entity; write a `ScopeFunc` directly for per-entity columns.
- **Explicit exemptions.** A `ScopeFunc` may return `(nil, nil)` to exempt a
  public entity.
- **Require a scope.** `RequireRowScope` refuses to compile when no `RowScope` is
  configured, so no query is ever unscoped by omission.
- **Mutations too.** `RowScope` is ANDed into UPDATE/DELETE `WHERE` (in addition
  to the required user filter), so a trusted mutation cannot touch another
  tenant's rows. On INSERT the scope's equality terms are stamped onto the new
  row (see below).
- **Hidden scope columns.** The scope predicate resolves against the full
  schema, so it may reference a column the caller cannot see, such as an
  internal-class `tenant_id`. The user's own query still cannot select or filter
  on that column.
- **Scope columns are not assignable.** `Insert` and `Update` reject an
  assignment to any column referenced by the scope, even if it is `Writable`, so
  a row cannot be created in or moved to another scope.

`RowScope` is per-request session state and is not serialized with `Config`.
Combine it with per-tenant schemas (column isolation) and database row-level
security (a final backstop) for defense in depth.

!!! note "INSERT"
    An INSERT has no `WHERE`, so the scope is applied by stamping: each equality
    term of the scope (`tenant_id = 5`, or an `And` of equalities) is added as a
    column/value pair. A scope that is not a conjunction of equalities (for
    example `IN` or `OR`) cannot be stamped, so the INSERT is refused rather than
    written unscoped. An entity the scope exempts is inserted unchanged.
