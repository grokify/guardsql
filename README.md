# GrokifyQL

[![Go CI][go-ci-svg]][go-ci-url]
[![Go Lint][go-lint-svg]][go-lint-url]
[![Go SAST][go-sast-svg]][go-sast-url]
[![Docs][docs-godoc-svg]][docs-godoc-url]
[![Visualization][viz-svg]][viz-url]
[![License][license-svg]][license-url]

 [go-ci-svg]: https://github.com/grokify/grokifyql/actions/workflows/go-ci.yaml/badge.svg?branch=main
 [go-ci-url]: https://github.com/grokify/grokifyql/actions/workflows/go-ci.yaml
 [go-lint-svg]: https://github.com/grokify/grokifyql/actions/workflows/go-lint.yaml/badge.svg?branch=main
 [go-lint-url]: https://github.com/grokify/grokifyql/actions/workflows/go-lint.yaml
 [go-sast-svg]: https://github.com/grokify/grokifyql/actions/workflows/go-sast-codeql.yaml/badge.svg?branch=main
 [go-sast-url]: https://github.com/grokify/grokifyql/actions/workflows/go-sast-codeql.yaml
 [docs-godoc-svg]: https://pkg.go.dev/badge/github.com/grokify/grokifyql
 [docs-godoc-url]: https://pkg.go.dev/github.com/grokify/grokifyql
 [docs-mkdoc-svg]: https://img.shields.io/badge/Go-dev%20guide-blue.svg
 [docs-mkdoc-url]: https://grokify.github.io/grokifyql
 [viz-svg]: https://img.shields.io/badge/visualizaton-Go-blue.svg
 [viz-url]: https://mango-dune-07a8b7110.1.azurestaticapps.net/?repo=grokify%2Fgrokifyql
 [loc-svg]: https://tokei.rs/b1/github/grokify/grokifyql
 [repo-url]: https://github.com/grokify/grokifyql
 [license-svg]: https://img.shields.io/badge/license-MIT-blue.svg
 [license-url]: https://github.com/grokify/grokifyql/blob/main/LICENSE

GrokifyQL is a small, application-neutral query language for Grokify analytics services. It is intended to provide a Jira Query Language style interface for SaaS reporting without passing customer-authored SQL directly to databases.

This repository is the Go engine for GrokifyQL: parser, AST, schema validation,
policy checks, lightweight authorization adapters, and backend compiler
contracts. Runtime integrations for other language stacks should live in
separate repositories or modules. A Hibernate integration, for example, should
belong in a Java-focused project such as `github.com/grokify/grokifyql-java` or
`github.com/grokify/grokifyql-hibernate`, not in this root Go module.

The core contract is a parser that converts user-authored text into Go structs. Applications should pass the parsed AST through schema validation, static lint policy, and backend-specific compilers. The original text should not be executed directly.

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

Future parsers may support create, update, or delete operations. Those operations are represented explicitly in the AST and must be allowed by policy before execution.

## Formatting

GrokifyQL includes a dependency-free formatter for canonical query display,
editor cleanup, logging, reviews, and tests.

```go
formatted, err := grokifyql.Format(input, grokifyql.DefaultFormatOptions())
```

The formatter supports multiline and single-line output:

```go
singleLine, err := grokifyql.Format(input, grokifyql.FormatOptions{
    Style: grokifyql.FormatSingleLine,
})
```

The `grokifyqlfmt` command reads a file or stdin:

```bash
go run ./cmd/grokifyqlfmt query.gql
echo 'select id,name from items limit 10' | go run ./cmd/grokifyqlfmt --style=singleline
echo 'select id from items' | go run ./cmd/grokifyqlfmt --highlight=ansi
```

The TypeScript package exports matching browser-side helpers:

```ts
import { format, highlightANSI } from '@grokify/grokifyql'

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
_, issues := grokifyql.Lint(input, grokifyql.LintConfig{
	Schema:     schema,
	AllowedOps: []grokifyql.Operation{grokifyql.OperationRead},
	MaxDepth:   8,
	MaxNodes:   64,
	MaxInValues: 100,
	RequireLimit: true,
})
```

New code should prefer parsing once, validating, analyzing requirements, and
applying both structural and resolved policy checks:

```go
q, err := grokifyql.Parse(input)
if err != nil {
    return err
}

issues := grokifyql.CheckPolicy(q, grokifyql.Policy{
    AllowedOps: []grokifyql.Operation{grokifyql.OperationRead},
    Fields: map[string]map[string]grokifyql.FieldPolicy{
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

analysis, err := grokifyql.Analyze(q, schema)
if err != nil {
    return err
}

requirements := analysis.Requirements()
_ = requirements

policy := grokifyql.SafeAnalyticsPolicy(schema)
if issues := grokifyql.CheckAnalysisPolicy(analysis, policy); len(issues) > 0 {
    return fmt.Errorf("query requirements not allowed: %s", issues[0].Message)
}
```

## Security Model

GrokifyQL should not be executed as SQL. Applications parse it into an AST, validate that AST against an allowlisted schema, then compile it into backend-specific parameterized queries, Ent predicates, API filters, or in-memory filters. Tenant scope and authorization should be injected by the hosting service, not trusted from the user query.

Input that does not parse is a normal syntax error, not an injection. Injection resistance comes from never executing the original string as SQL and from only compiling validated AST nodes to parameterized backend calls.

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

## Authorization Engines

The core module is intentionally dependency-light. It defines the policy shape
and AST checks, but it does not import Cedar, Rego, SpiceDB, Casbin, or other
authorization engines.

External authorization systems should make decisions from trusted application
context and compile those decisions into `grokifyql.Policy`. The lightweight
`github.com/grokify/grokifyql/authz` package provides a `Decision` helper for
that conversion.

For SystemForge and SpiceDB integrations, use the optional nested module:

```bash
go get github.com/grokify/grokifyql/authzsystemforge
```

`authzsystemforge.PolicyBuilder` accepts a SystemForge `authz.Authorizer`, a
principal, a GrokifyQL schema, and an optional resource mapper. It checks
entities and fields through SystemForge, then produces a read-only
`grokifyql.Policy`.

Production SpiceDB deployments should provide a custom resource mapper so
GrokifyQL entities and fields map to concrete product resources such as
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
