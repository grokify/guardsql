# Mutations

GuardSQL supports trusted, application-authored mutations (INSERT/UPDATE/DELETE)
through a programmatic builder. Trusted code constructs a statement from typed
parts — it does **not** parse mutation SQL text — and compiles it through the
same hardened path as reads: values are bound as parameters, identifiers are
resolved against the schema and re-quoted, and the operation must be permitted
by policy.

Use mutations only with a trusted [Config](trust-modes.md); customer-supplied
SQL must never reach this path.

Assignment targets must be columns marked `Writable` in the schema (a
deny-by-default allowlist, orthogonal to the read capabilities):

```go
"status": {Type: guardsql.FieldString, Selectable: true, Writable: true},
"id":     {Type: guardsql.FieldNumber, Selectable: true}, // read-only, not assignable
```

## Builders

```go
cfg := guardsql.NewTrustedConfig(schema, guardsql.DialectPostgres)

// INSERT
guardsql.Insert("users").
    Set("email", email).
    Set("status", "active").
    Compile(cfg)
// INSERT INTO "users" ("email", "status") VALUES ($1, $2)   args: [email, "active"]

// UPDATE
guardsql.Update("users").
    Set("status", "active").
    Where(guardsql.Eq("id", 42)).
    Compile(cfg)
// UPDATE "users" SET "status" = $1 WHERE "id" = $2          args: ["active", 42]

// DELETE
guardsql.Delete("users").
    Where(guardsql.And(guardsql.Eq("status", "disabled"), guardsql.Lt("id", 100))).
    Compile(cfg)
// DELETE FROM "users" WHERE ("status" = $1 AND "id" < $2)   args: ["disabled", 100]
```

## Condition Helpers

Build `WHERE` clauses with typed helpers; every value becomes a bind parameter.

| Helper | SQL |
|--------|-----|
| `Eq(f, v)` / `Ne(f, v)` | `f = v` / `f != v` |
| `Lt` / `Lte` / `Gt` / `Gte` | `f < v`, `f <= v`, `f > v`, `f >= v` |
| `In(f, v...)` | `f IN (...)` |
| `IsNull(f)` / `IsNotNull(f)` | `f IS NULL` / `f IS NOT NULL` |
| `And(...)` / `Or(...)` / `Not(e)` | boolean composition |

## Safety Rules

- **Deny by default.** A mutation is compiled only if its operation is
  explicitly listed in the Config's `Policy.AllowedOps`. A zero `Config` and
  `NewUntrustedConfig` both permit no mutations; only `NewTrustedConfig` (or an
  explicit `AllowedOps`) enables them.
- **`WHERE` required.** `Update` and `Delete` reject a missing `WHERE` unless
  `Config.AllowUnfilteredMutations` is set — no accidental full-table writes.
- **Parameterized values.** A value such as `x'; DROP TABLE users; --` is a
  bound argument, never inlined SQL.
- **Writable allowlist (mass-assignment defense).** An assignment target must be
  a column explicitly marked `Writable` in the schema. `Writable` is
  deny-by-default, so a caller cannot write a column such as `role` or
  `is_admin` unless the schema permits it — even when the field is readable.
- **Schema-checked identifiers.** Unknown columns are rejected; assignment
  targets must be plain columns (not JSON paths); identifier lockdown applies.

## Why a Builder Instead of Parsed SQL

For internal calls, constructing typed AST nodes is safer than parsing SQL
strings: there is no mutation grammar to attack, and the compiler guarantees
parameterization and identifier safety. A mutation SQL parser may be added later
if routing existing trusted SQL strings through GuardSQL becomes useful.
