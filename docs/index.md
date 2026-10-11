# GuardSQL

GuardSQL is a small, application-neutral query language for SaaS analytics and
reporting. It gives users a SQL-like interface without passing customer-authored
SQL directly to a database.

The core contract is:

```text
query text -> parse -> validate -> policy -> compile -> parameterized SQL + args
```

Untrusted SQL in, trusted SQL out: GuardSQL parses customer text into an AST,
validates and policy-checks it, then **regenerates** parameterized,
dialect-specific SQL from the AST — the original text is never executed.
GuardSQL owns parsing, AST types, validation, policy checks, a built-in SQL
compiler (SQLite/PostgreSQL/MySQL with JSON extraction), a trusted mutation
builder, and a limited in-memory evaluator. Applications own tenant scope,
authorization, execution limits, and audit logging.

This repository is the Go implementation of that engine. Language/runtime
integrations should be separate from the root module. For example, a Hibernate
adapter should live in a Java-oriented repository such as
`github.com/grokify/guardsql-java` or `github.com/grokify/guardsql-hibernate`
and consume the same conceptual AST/policy contract without adding Java
dependencies here.

## Design Goals

- Keep customer query text away from direct database execution.
- Regenerate parameterized SQL from the validated AST, never pass user text
  through.
- Make unsupported syntax fail as a normal parse error.
- Validate entities and fields against explicit allowlisted schemas.
- Enforce read-only and field-level policy against the AST.
- Serve trusted internal SQL (including mutations) from the same hardened engine.
- Keep the root Go module dependency-light.
- Put heavyweight authorization integrations in optional modules.
- Keep non-Go runtime integrations outside this repository.

## Capabilities

- **Parser + AST** for a read-only SQL subset.
- **Schema validation** against an allowlist of entities, fields, types, and
  capabilities, with per-field [data classification](guides/trust-modes.md#data-classification).
- **Policy** checks for operations, limits, and query complexity.
- **[SQL compiler](guides/compiler.md)** — AST to parameterized SQLite / PostgreSQL /
  MySQL, including JSON path extraction.
- **[Config & trust modes](guides/trust-modes.md)** — hardened untrusted and trusted
  profiles with a one-step `Compile`.
- **[Mutations](guides/mutations.md)** — a trusted INSERT/UPDATE/DELETE builder.
- **[Identifier lockdown](guides/trust-modes.md#identifier-safety)** — safe-charset
  enforcement for table and column names.

## Core Packages

| Package | Purpose |
|---------|---------|
| `github.com/grokify/guardsql` | Parser, AST, schema validation, policy, evaluator |
| `github.com/grokify/guardsql/authz` | Lightweight authorization decision helper |
| `github.com/grokify/guardsql/authzsystemforge` | Optional SystemForge/SpiceDB adapter module |

## Minimal Example

The high-level `Config` runs parse → validate → policy → compile in one call:

```go
cfg := guardsql.NewUntrustedConfig(schema, guardsql.DialectPostgres)

out, err := cfg.Compile(`SELECT id, name FROM roadmap_items WHERE status = "planned" LIMIT 25`)
if err != nil {
    return err
}
rows, err := db.Query(out.SQL, out.Args...) // parameterized, dialect-specific
```

The lower-level primitives remain available for custom pipelines:

```go
q, err := guardsql.Parse(input)
if err != nil {
    return err
}
if err := guardsql.Validate(q, schema); err != nil {
    return err
}
if issues := guardsql.CheckPolicy(q, policy); len(issues) > 0 {
    return fmt.Errorf("query not allowed: %s", issues[0].Message)
}
out, err := guardsql.Compile(q, guardsql.CompileOptions{Dialect: guardsql.DialectPostgres, Schema: schema})
```
