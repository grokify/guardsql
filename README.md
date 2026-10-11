# GuardSQL

[![Go CI][go-ci-svg]][go-ci-url]
[![Go Lint][go-lint-svg]][go-lint-url]
[![Go SAST][go-sast-svg]][go-sast-url]
[![Docs][docs-godoc-svg]][docs-godoc-url]
[![Visualization][viz-svg]][viz-url]
[![License][license-svg]][license-url]

 [go-ci-svg]: https://github.com/grokify/guardsql/actions/workflows/go-ci.yaml/badge.svg?branch=main
 [go-ci-url]: https://github.com/grokify/guardsql/actions/workflows/go-ci.yaml
 [go-lint-svg]: https://github.com/grokify/guardsql/actions/workflows/go-lint.yaml/badge.svg?branch=main
 [go-lint-url]: https://github.com/grokify/guardsql/actions/workflows/go-lint.yaml
 [go-sast-svg]: https://github.com/grokify/guardsql/actions/workflows/go-sast-codeql.yaml/badge.svg?branch=main
 [go-sast-url]: https://github.com/grokify/guardsql/actions/workflows/go-sast-codeql.yaml
 [docs-godoc-svg]: https://pkg.go.dev/badge/github.com/grokify/guardsql
 [docs-godoc-url]: https://pkg.go.dev/github.com/grokify/guardsql
 [docs-mkdoc-svg]: https://img.shields.io/badge/Go-dev%20guide-blue.svg
 [docs-mkdoc-url]: https://grokify.github.io/guardsql
 [viz-svg]: https://img.shields.io/badge/visualizaton-Go-blue.svg
 [viz-url]: https://mango-dune-07a8b7110.1.azurestaticapps.net/?repo=grokify%2Fguardsql
 [loc-svg]: https://tokei.rs/b1/github/grokify/guardsql
 [repo-url]: https://github.com/grokify/guardsql
 [license-svg]: https://img.shields.io/badge/license-MIT-blue.svg
 [license-url]: https://github.com/grokify/guardsql/blob/main/LICENSE

GuardSQL is a small, application-neutral query language for Grokify analytics services. It is intended to provide a Jira Query Language style interface for SaaS reporting without passing customer-authored SQL directly to databases.

This repository is the Go engine for GuardSQL: parser, AST, schema validation, policy checks, a built-in SQL compiler (SQLite/PostgreSQL/MySQL with JSON extraction), a trusted mutation builder, and lightweight authorization adapters. Runtime integrations for other language stacks should live in separate repositories or modules.

GuardSQL is a **transpiler**: untrusted SQL in, trusted parameterized SQL out. It parses user-authored text into a Go AST, validates and policy-checks that AST, then **regenerates** dialect-specific parameterized SQL from it. The original text is never executed — only constructs the AST can represent and the compiler knows how to emit can reach the database, which makes the accepted surface default-deny by construction.

```text
query text -> parse -> validate -> policy -> compile -> parameterized SQL + args
```

For an end-to-end walkthrough (schema, untrusted analytics with row scope, and
trusted mutations), see [Getting Started](docs/getting-started.md); for why the
model is secure and what the host must still enforce, see
[Security Hardening](docs/security-hardening.md).

## Scope

The first parser is read-only and produces `OperationRead` queries:

```sql
SELECT id, name, priority
FROM roadmap_items
WHERE status IN ("planned", "in_progress") AND score >= 50
ORDER BY score DESC
LIMIT 25
```

Supported clauses:

- `SELECT ... FROM ...`
- `WHERE` with `AND`, `OR`, `NOT`, parentheses, `=`, `!=`, `<`, `<=`, `>`, `>=`, `IN`, `CONTAINS`, `IS NULL`, and `IS NOT NULL`
- `GROUP BY`
- `HAVING`
- `ORDER BY`
- `LIMIT`
- `WITH` common table expressions
- `JOIN`, `INNER JOIN`, `LEFT JOIN`, `LEFT OUTER JOIN`, `RIGHT JOIN`, `RIGHT OUTER JOIN`, `FULL JOIN`, `FULL OUTER JOIN`, and `CROSS JOIN`
- nested query sources in `FROM (...)`

Supported aggregate functions:

- `COUNT(*)`
- `COUNT(field)`
- `SUM(field)`
- `AVG(field)`
- `MIN(field)`
- `MAX(field)`

The in-memory evaluator supports filters, grouping, aggregates, `HAVING`, ordering, limits, and projection for single-source queries. Joins, CTEs, and nested query sources are represented in the AST and should be executed by backend-specific compilers. Self joins are represented as normal joins and require aliases on both sources.

## Compiling to SQL

GuardSQL ships a compiler that transpiles a validated AST into parameterized,
dialect-specific SQL (SQLite, PostgreSQL, MySQL/Dolt). Values become bind
parameters; identifiers are re-resolved against the schema and re-quoted.

```go
out, err := guardsql.Compile(q, guardsql.CompileOptions{
    Dialect: guardsql.DialectPostgres, Schema: schema, StrictIdentifiers: true,
})
// out.SQL, out.Args -> db.Query(out.SQL, out.Args...)
```

A `FieldJSON` column can be addressed with a dotted path, compiled to
dialect-specific JSON extraction:

```sql
SELECT payload.finding_info.title AS t, count(*) AS n
FROM events GROUP BY payload.finding_info.title
-- SQLite: json_extract("payload", ?) ...  args: ["$.finding_info.title", ...]
```

See the [SQL Compiler](docs/compiler.md) docs.

## Config & Trust Modes

`Config` is the recommended entry point: parse → validate → policy → compile in
one call, with secure defaults. The same engine serves customer-supplied SQL and
internal application SQL through two profiles:

```go
// Customer SQL: read-only, secret/PII columns stripped, identifiers locked, LIMIT required.
cfg := guardsql.NewUntrustedConfig(schema, guardsql.DialectSQLite)
out, err := cfg.Compile(customerQuery)

// Internal SQL: reads + mutations, full column access, still parameterized and locked.
tcfg := guardsql.NewTrustedConfig(schema, guardsql.DialectPostgres)
```

Each schema field carries a data classification — `ClassPublic` (default),
`ClassPII`, `ClassInternal`, or `ClassSecret`. Classified columns are stripped
from the effective schema unless `AllowPII` / `AllowInternal` / `AllowSecret`
are set, so an untrusted query that references one fails as an unknown field.
See [Config & Trust Modes](docs/trust-modes.md).

The recommended source of classification is a
[`datacatalog`](https://github.com/grokify/datacatalog) catalog:

```go
schema, err := guardsql.FromCatalog(catalog) // validates, then maps visibility/pii/secret
cfg := guardsql.NewUntrustedConfig(schema, guardsql.DialectPostgres)
```

`Field.Class` remains settable by hand. See
[Schema from a Data Catalog](docs/catalog-schema.md).

For tenant/ownership isolation (IDOR defense), `Config.RowScope` injects a
mandatory, app-supplied predicate as a top-level `AND` on every compiled
SELECT/UPDATE/DELETE and is stamped onto INSERTs, so no user `WHERE` can widen
scope and scope columns cannot be reassigned; `RequireRowScope` makes unscoped
access fail closed. See
[Row Scope](docs/trust-modes.md#row-scope-tenant--idor-isolation).

## Identifier Safety

Table and column names cannot be bound as parameters, so GuardSQL restricts them
to the unquoted identifier charset by default. `IsSafeIdentifier` and
`Schema.ValidateIdentifiers()` reject quotes, whitespace, `;`, `--`, and
non-ASCII homoglyphs before any SQL is emitted, making identifier-based injection
structurally impossible. Set `AllowUnsafeIdentifiers` only for a database that
relies on quoted identifiers.

## Mutations

Trusted, application-authored mutations use a programmatic builder — no mutation
text is parsed. They are deny-by-default (the operation must be allowed by
policy) and require a `WHERE` on UPDATE/DELETE unless
`AllowUnfilteredMutations` is set.

```go
guardsql.Update("users").
    Set("status", "active").
    Where(guardsql.Eq("id", 42)).
    Compile(tcfg)
// UPDATE "users" SET "status" = $1 WHERE "id" = $2   args: ["active", 42]
```

See [Mutations](docs/mutations.md).

## Formatting

GuardSQL includes a dependency-free formatter for canonical query display,
editor cleanup, logging, reviews, and tests.

```go
formatted, err := guardsql.Format(input, guardsql.DefaultFormatOptions())
```

The formatter supports multiline and single-line output:

```go
singleLine, err := guardsql.Format(input, guardsql.FormatOptions{
    Style: guardsql.FormatSingleLine,
})
```

The `guardsqlfmt` command reads a file or stdin:

```bash
go run ./cmd/guardsqlfmt query.gql
echo 'select id,name from items limit 10' | go run ./cmd/guardsqlfmt --style=singleline
echo 'select id from items' | go run ./cmd/guardsqlfmt --highlight=ansi
```

The TypeScript package exports matching browser-side helpers:

```ts
import { format, highlightANSI } from '@grokify/guardsql'

format('select id,name from items limit 10')
```

## Request Pipeline

Hosted applications should process customer queries in this order:

1. `Parse`: convert text into a `Query` AST. Anything that does not parse is a syntax error.
2. `Validate`: check the AST against an allowlisted schema of entities, fields, field types, and allowed field capabilities.
3. `Analyze`: resolve sources and fields into a `RequirementSet` for authorization and backend planning.
4. `Policy`: enforce static product policy such as allowed CRUD operations, required limits, expression complexity, maximum `IN` list size, and field usage.
5. `Authorize`: apply tenant, user, role, and row-level authorization outside the user query, or compile those decisions into `Policy`.
6. `Compile`: convert the validated AST into backend-specific parameterized SQL, Ent predicates, API filters, or in-memory filters.
7. `Execute`: run with service-owned timeout, row limit, audit logging, and resource controls.

For analytics/reporting endpoints, configure linting with read-only operation policy:

```go
_, issues := guardsql.Lint(input, guardsql.LintConfig{
	Schema:     schema,
	AllowedOps: []guardsql.Operation{guardsql.OperationRead},
	MaxDepth:   8,
	MaxNodes:   64,
	MaxInValues: 100,
	RequireLimit: true,
})
```

New code should prefer parsing once, validating, analyzing requirements, and
applying both structural and resolved policy checks:

```go
q, err := guardsql.Parse(input)
if err != nil {
    return err
}

issues := guardsql.CheckPolicy(q, guardsql.Policy{
    AllowedOps: []guardsql.Operation{guardsql.OperationRead},
    Fields: map[string]map[string]guardsql.FieldPolicy{
        "roadmap_items": {
            "name":  {Selectable: true, Filterable: true, Sortable: true},
            "score": {Selectable: true, Filterable: true, Sortable: true},
        },
    },
    MaxDepth:    8,
    MaxNodes:    80,
    MaxInValues: 100,
})
if len(issues) > 0 {
    return fmt.Errorf("query not allowed: %s", issues[0].Message)
}

analysis, err := guardsql.Analyze(q, schema)
if err != nil {
    return err
}

requirements := analysis.Requirements()
_ = requirements

policy := guardsql.SafeAnalyticsPolicy(schema)
if issues := guardsql.CheckAnalysisPolicy(analysis, policy); len(issues) > 0 {
    return fmt.Errorf("query requirements not allowed: %s", issues[0].Message)
}
```

## Security Model

GuardSQL is never executed as SQL. Applications parse it into an AST, validate that AST against an allowlisted schema, then compile it — with the built-in compiler or a backend-specific one — into parameterized queries, Ent predicates, API filters, or in-memory filters. Tenant scope and authorization should be injected by the hosting service, not trusted from the user query.

Input that does not parse is a normal syntax error, not an injection. Injection resistance comes from never executing the original string as SQL and from only compiling validated AST nodes to parameterized backend calls. The built-in compiler additionally binds every value as a parameter and locks identifiers to a safe charset (see Identifier Safety above).

For multi-tenant SaaS services, treat the AST as the trust boundary:

- Use per-tenant and per-role schemas so users cannot reference unauthorized entities or fields.
- Force tenant scope outside the query instead of accepting `tenant_id` from user text.
- Allow only `OperationRead` on analytics endpoints.
- Keep mutations behind separate endpoints, policies, audit logs, and permission checks.
- Compile SQL with placeholders and separate arguments, never string-concatenated user values.
- Apply service-side query timeouts and row limits even when the query includes `LIMIT`.
- Use least-privileged read-only database roles, row-level security or approved
  tenant-scoped views, and deny database file/network/procedure/extension
  privileges for customer query execution.

## Attack Coverage

Strength ratings assume the compiled path (`Compile` / `Config.Compile`). Using
GuardSQL only as a parse/policy gate and then executing the original text
verbatim weakens the injection guarantees to whatever the parser rejects.

- **Strong** — prevented by construction.
- **Partial** — reduced at the language layer; close fully with host/database controls.
- **Shared** — GuardSQL enforces a decision the application must make.
- **App-layer** — outside GuardSQL's scope.

| Attack class | Strength | GuardSQL posture |
|--------------|----------|------------------|
| SQL injection — values | Strong | Every literal is a bind parameter; values are never concatenated into SQL. |
| SQL injection — identifiers | Strong | Table/column names must resolve to a schema field and pass `IsSafeIdentifier`, then are quoted per dialect. |
| Blind / boolean / timing injection | Strong | Parameterized values plus no scalar functions or arithmetic in the grammar leave no oracle to build. |
| Stacked queries, DDL, procedures, file/network access | Strong | Not representable in the AST, so the compiler cannot emit them. |
| Unicode / homoglyph identifier spoofing | Strong | `IsSafeIdentifier` is ASCII-only and rejects non-identifier characters. |
| Second-order injection | App-layer | Values are stored safely as parameters; if other code later concatenates a stored value into SQL, that path is outside GuardSQL. |
| Query-complexity denial of service | Partial | Policy bounds depth, node count, `IN` size, `LIMIT`, and select/join/group/CTE counts; expensive-but-legal queries still need DB timeouts, row caps, and a least-privileged role. |
| Column-level data exposure (secrets / PII) | Strong | Entity/field allowlist plus `ClassSecret`/`ClassInternal`/`ClassPII` stripping; denied columns read as "unknown field" without disclosing they exist. |
| Mass assignment via mutations | Strong | Assignment targets must be columns explicitly marked `Writable` in the schema (deny-by-default), so a caller cannot write a column such as `role` or `is_admin` unless permitted; mutations are also trusted-only and reject unknown columns. |
| Full-table `UPDATE` / `DELETE` | Strong | A `WHERE` clause is required by default; unfiltered mutations are refused unless explicitly allowed. |
| Error-based information disclosure | Strong | Errors are structural; denied/secret columns surface as unknown fields rather than confirming existence. |
| Broken object-level authorization (IDOR) | Strong&nbsp;* | The app supplies the trusted scope (session principal → row filter); GuardSQL injects it as a mandatory top-level `AND` on every SELECT/UPDATE/DELETE via `RowScope`, so no user `WHERE` can widen it. `RequireRowScope` makes unscoped access refuse to compile (fail-closed). |
| Multi-tenant data leakage | Strong&nbsp;* | Same mechanism: a forced `tenant_id` predicate is ANDed into every statement, and per-tenant schemas add column-level isolation. |
| Authorization bypass | Shared | External authz engines compile decisions into `guardsql.Policy` (`authz.PolicyFromDecision`); GuardSQL enforces the compiled decision against the AST. |

\* Strong **when the application configures `RowScope`** from trusted session
context — GuardSQL enforces the scope but cannot know the session tenant/owner
on its own. Without it, row-level isolation is not enforced.

The database account remains the final boundary for resource and RCE risk: run
untrusted queries against a read-only, least-privileged identity with row-level
security or approved tenant-scoped views.

## Authorization Engines

The core module is intentionally dependency-light. It defines the policy shape
and AST checks, but it does not import Cedar, Rego, SpiceDB, Casbin, or other
authorization engines.

External authorization systems should make decisions from trusted application
context and compile those decisions into `guardsql.Policy`. The lightweight
`github.com/grokify/guardsql/authz` package provides a `Decision` helper for
that conversion.

For SystemForge and SpiceDB integrations, use the optional nested module:

```bash
go get github.com/grokify/guardsql/authzsystemforge
```

`authzsystemforge.PolicyBuilder` accepts a SystemForge `authz.Authorizer`, a
principal, a GuardSQL schema, and an optional resource mapper. It checks
entities and fields through SystemForge, then produces a read-only
`guardsql.Policy`.

Production SpiceDB deployments should provide a custom resource mapper so
GuardSQL entities and fields map to concrete product resources such as
`analytics_dataset:<id>` and `analytics_field:<id>`.

## Dialects

`AQL` for Aha Studio and `ORQL` for OmniRoadmap can be exposed as product dialect names while using this shared package underneath.

## Documentation

This repo includes a MkDocs site in `docs/`.

```bash
mkdocs serve
mkdocs build --strict
```

## TypeScript Package

The browser-side package lives in `ts/` and is intended for UI feedback such as
syntax errors, catalog validation, field policy warnings, and query-builder
helpers. The Go parser and backend policy checks remain authoritative.

```bash
cd ts
npm test
```
